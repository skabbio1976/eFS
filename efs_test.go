package efs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestExtractToTempAndCleanup(t *testing.T) {
	mem := fstest.MapFS{
		"root/a.txt":    {Data: []byte("A")},
		"root/sub/b.js": {Data: []byte("B")},
	}

	dir, cleanup, err := ExtractToTemp(mem, "root", "tst")
	if err != nil {
		t.Fatalf("ExtractToTemp error: %v", err)
	}
	defer cleanup()

	// Files should exist directly under temp, without extra top-level "root" dir
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatalf("expected a.txt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "b.js")); err != nil {
		t.Fatalf("expected sub/b.js: %v", err)
	}

	// Cleanup should remove dir
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected dir removed, got err=%v", err)
	}
}

func TestExtractRootDot(t *testing.T) {
	mem := fstest.MapFS{
		"a.txt": {Data: []byte("A")},
	}
	dir, cleanup, err := ExtractToTemp(mem, ".", "tst")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatalf("expected a.txt: %v", err)
	}
}

func TestExtractEmptyRootDefaultsToDot(t *testing.T) {
	mem := fstest.MapFS{"a.txt": {Data: []byte("A")}}
	dir, cleanup, err := ExtractToTemp(mem, "", "tst")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatalf("expected a.txt: %v", err)
	}
}

// badFS injects an error for a particular path when opening.
type badFS struct {
	base fstest.MapFS
	fail string
}

func (b badFS) Open(name string) (fs.File, error) {
	if name == b.fail || (b.fail == "." && (name == "." || name == "")) {
		return nil, errors.New("forced open error")
	}
	return b.base.Open(name)
}

func TestErrorPropagates(t *testing.T) {
	// Force an error when opening the root directory to make WalkDir fail immediately
	bad := badFS{base: fstest.MapFS{"a.txt": {Data: []byte("A")}}, fail: "."}
	dir, cleanup, err := ExtractToTemp(bad, ".", "tst")
	if err == nil {
		t.Fatalf("expected error, got none (dir=%q)", dir)
	}
	if dir != "" || cleanup != nil {
		t.Fatalf("expected empty dir and nil cleanup on error; got dir=%q cleanup non-nil=%t", dir, cleanup != nil)
	}
}

func TestConcurrentExtractions(t *testing.T) {
	mem := fstest.MapFS{
		"a.txt":     {Data: []byte("A")},
		"sub/b.txt": {Data: []byte("B")},
	}

	const numGoroutines = 10
	done := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			dir, cleanup, err := ExtractToTemp(mem, ".", "concurrent")
			if err != nil {
				done <- err
				return
			}
			defer cleanup()

			// Verify files exist
			if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
				done <- err
				return
			}
			if _, err := os.Stat(filepath.Join(dir, "sub", "b.txt")); err != nil {
				done <- err
				return
			}

			done <- nil
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		if err := <-done; err != nil {
			t.Errorf("goroutine failed: %v", err)
		}
	}
}

func TestLargeFile(t *testing.T) {
	// Create a 10MB file to test memory handling
	const fileSize = 10 * 1024 * 1024 // 10MB
	largeData := make([]byte, fileSize)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	mem := fstest.MapFS{
		"large.bin": {Data: largeData},
		"small.txt": {Data: []byte("small")},
	}

	dir, cleanup, err := ExtractToTemp(mem, ".", "large")
	if err != nil {
		t.Fatalf("ExtractToTemp error: %v", err)
	}
	defer cleanup()

	// Verify large file exists and has correct size
	largePath := filepath.Join(dir, "large.bin")
	info, err := os.Stat(largePath)
	if err != nil {
		t.Fatalf("large file not found: %v", err)
	}
	if info.Size() != fileSize {
		t.Errorf("expected size %d, got %d", fileSize, info.Size())
	}

	// Verify small file still works
	smallPath := filepath.Join(dir, "small.txt")
	if _, err := os.Stat(smallPath); err != nil {
		t.Fatalf("small file not found: %v", err)
	}
}

func TestDeepDirectoryHierarchy(t *testing.T) {
	// Create a deep directory structure (20 levels deep)
	mem := fstest.MapFS{
		"a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/deep.txt": {Data: []byte("deep file")},
		"a/b/c/mid.txt":                                     {Data: []byte("mid level")},
		"a/shallow.txt":                                     {Data: []byte("shallow")},
	}

	dir, cleanup, err := ExtractToTemp(mem, ".", "deep")
	if err != nil {
		t.Fatalf("ExtractToTemp error: %v", err)
	}
	defer cleanup()

	// Verify deep file exists
	deepPath := filepath.Join(dir, "a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/deep.txt")
	data, err := os.ReadFile(deepPath)
	if err != nil {
		t.Fatalf("deep file not found: %v", err)
	}
	if string(data) != "deep file" {
		t.Errorf("expected 'deep file', got %q", string(data))
	}

	// Verify mid-level file
	midPath := filepath.Join(dir, "a/b/c/mid.txt")
	if _, err := os.Stat(midPath); err != nil {
		t.Fatalf("mid-level file not found: %v", err)
	}

	// Verify shallow file
	shallowPath := filepath.Join(dir, "a/shallow.txt")
	if _, err := os.Stat(shallowPath); err != nil {
		t.Fatalf("shallow file not found: %v", err)
	}
}

func TestSymlinkHandling(t *testing.T) {
	// Create a temporary directory with actual files and symlinks
	sourceDir, err := os.MkdirTemp(".", "symlink-source-")
	if err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	defer os.RemoveAll(sourceDir)

	// Create a regular file
	regularFile := filepath.Join(sourceDir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("regular content"), 0o644); err != nil {
		t.Fatalf("failed to create regular file: %v", err)
	}

	// Create a symlink to the regular file using relative path
	symlinkFile := filepath.Join(sourceDir, "link.txt")
	if err := os.Symlink("regular.txt", symlinkFile); err != nil {
		t.Skipf("symlink creation not supported: %v", err)
	}

	// Use os.DirFS to read the directory with symlinks
	fsys := os.DirFS(sourceDir)

	dir, cleanup, err := ExtractToTemp(fsys, ".", "symlink")
	if err != nil {
		t.Fatalf("ExtractToTemp error: %v", err)
	}
	defer cleanup()

	// Verify regular file exists
	extractedRegular := filepath.Join(dir, "regular.txt")
	data, err := os.ReadFile(extractedRegular)
	if err != nil {
		t.Fatalf("regular file not found: %v", err)
	}
	if string(data) != "regular content" {
		t.Errorf("expected 'regular content', got %q", string(data))
	}

	// Check if symlink was extracted (it will be extracted as the target file's content)
	// Note: fs.ReadFile follows symlinks, so the extracted file will be a regular file
	extractedLink := filepath.Join(dir, "link.txt")
	linkData, err := os.ReadFile(extractedLink)
	if err != nil {
		t.Fatalf("symlink file not found: %v", err)
	}
	if string(linkData) != "regular content" {
		t.Errorf("expected symlink to contain 'regular content', got %q", string(linkData))
	}
}
