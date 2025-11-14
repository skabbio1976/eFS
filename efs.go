// Package efs provides helpers to extract a directory or single files from an embedded
// or generic fs.FS into a temporary folder for easy runtime access, plus signal-aware cleanup helpers.
//
// Temp Directory Strategy:
//   - Each call to ExtractToTemp() or ExtractFile() creates a NEW temporary directory.
//   - If called 100 times, 100 separate temp directories will be created.
//   - Each temp directory has a unique name based on the prefix and a random suffix.
//   - It's the caller's responsibility to call cleanup() to remove temp directories.
//   - Use StartCleanupListener() to automatically clean up on program termination signals.
//   - By default, temp directories are created in the current working directory.
//   - You can specify a custom base directory using the tempDir parameter (empty string = default).
package efs

import (
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// ExtractToTemp walks the provided filesystem (embed.FS or any fs.FS) starting at
// the specified root path and extracts its contents into a new temporary directory.
//
// Parameters:
//   - fsys: The filesystem to extract from (embed.FS, fstest.MapFS, os.DirFS, etc.)
//   - root: The root path within fsys to extract (empty string defaults to ".")
//   - tempPrefix: Prefix for the temporary directory name
//   - tempDir: Base directory where temp dir will be created (empty string = current working directory)
//
// Behavior:
//   - If root is empty, "." is used.
//   - The returned temp directory contains the CONTENTS of root (the root folder
//     itself is not created inside the temp directory).
//   - Each call creates a NEW temporary directory with a unique name.
//   - Returns: absolute temp directory path, an idempotent cleanup func, and error.
//
// Example:
//
//	dir, cleanup, err := ExtractToTemp(assets, "assets", "myassets", "")
//	defer cleanup()
func ExtractToTemp(fsys fs.FS, root string, tempPrefix string, tempDir string) (string, func(), error) {
	if root == "" {
		root = "."
	}

	// Use current working directory if tempDir is empty
	baseDir := tempDir
	if baseDir == "" {
		baseDir = "."
	}

	// Create a temporary directory in the specified base directory
	temp, err := os.MkdirTemp(baseDir, tempPrefix+"-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	absTempDir, absErr := filepath.Abs(temp)
	if absErr != nil {
		// Fallback to relative path if Abs fails
		absTempDir = temp
	}

	// Idempotent cleanup
	var once sync.Once
	cleanup := func() {
		once.Do(func() { _ = os.RemoveAll(absTempDir) })
	}

	// Walk and extract
	err = fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Skip creating the top-level root dir inside temp; only its contents
		if path == root && d.IsDir() {
			return nil
		}

		// Build relative path (strip leading "root/" if root != ".")
		rel := path
		if root != "." && root != "" {
			if r, ok := strings.CutPrefix(path, root+"/"); ok {
				rel = r
			} else if path == root { // safety, though handled above
				rel = "."
			}
		}

		dst := filepath.Join(absTempDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}

		// Ensure parent dirs exist (robust even if Walk order changes)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		cleanup() // Clean up if extraction fails
		return "", nil, err
	}

	return absTempDir, cleanup, nil
}

// ExtractFile extracts a single file from the provided filesystem into a temporary file.
//
// Parameters:
//   - fsys: The filesystem to extract from (embed.FS, fstest.MapFS, os.DirFS, etc.)
//   - filePath: The path to the file within fsys to extract
//   - tempPrefix: Prefix for the temporary file name
//   - tempDir: Base directory where temp file will be created (empty string = current working directory)
//
// Behavior:
//   - Creates a new temporary file with a unique name.
//   - The file will have the same content as the source file.
//   - Returns: absolute path to the temp file, an idempotent cleanup func, and error.
//   - Each call creates a NEW temporary file.
//
// Example:
//
//	file, cleanup, err := ExtractFile(assets, "assets/config.json", "config", "")
//	defer cleanup()
func ExtractFile(fsys fs.FS, filePath string, tempPrefix string, tempDir string) (string, func(), error) {
	// Use current working directory if tempDir is empty
	baseDir := tempDir
	if baseDir == "" {
		baseDir = "."
	}

	// Read the file from the filesystem
	data, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return "", nil, fmt.Errorf("read file %q: %w", filePath, err)
	}

	// Create a temporary file
	// Extract extension from original filename if present
	ext := filepath.Ext(filePath)
	tempFile, err := os.CreateTemp(baseDir, tempPrefix+"-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}

	// Write data to temp file
	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", nil, fmt.Errorf("write temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempFile.Name())
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}

	absFilePath, absErr := filepath.Abs(tempFile.Name())
	if absErr != nil {
		// Fallback to relative path if Abs fails
		absFilePath = tempFile.Name()
	}

	// Idempotent cleanup
	var once sync.Once
	cleanup := func() {
		once.Do(func() { _ = os.Remove(absFilePath) })
	}

	return absFilePath, cleanup, nil
}

// StartCleanupListener starts a goroutine that listens for shutdown signals (e.g., Ctrl+C or SIGTERM)
// and cleans up the specified directory before exiting the program.
// It returns a stop function to disable the listener when you no longer need it.
// Note: os.Exit is called after cleanup, which skips other defers by design.
func StartCleanupListener(dir string) (stop func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	stopped := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			fmt.Printf("Received signal %v, cleaning up %s\n", sig, dir)
			if err := os.RemoveAll(dir); err != nil {
				fmt.Printf("Error cleaning up %s: %v\n", dir, err)
			}
			if s, ok := sig.(syscall.Signal); ok {
				os.Exit(128 + int(s))
			} else {
				os.Exit(1)
			}
		case <-stopped:
			return
		}
	}()

	return func() {
		close(stopped)
		signal.Stop(sigCh)
	}
}
