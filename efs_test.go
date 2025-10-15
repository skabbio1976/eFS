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
