// Package efs provides helpers to extract a directory from an embedded or generic fs.FS
// into a temporary folder for easy runtime access, plus signal-aware cleanup helpers.
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
// the specified root path and extracts its contents into a new temporary directory
// inside the current working directory.
//
// Behavior:
//   - If root is empty, "." is used.
//   - The returned temp directory contains the CONTENTS of root (the root folder
//     itself is not created inside the temp directory).
//   - Returns: absolute temp directory path, an idempotent cleanup func, and error.
func ExtractToTemp(fsys fs.FS, root string, tempPrefix string) (string, func(), error) {
	if root == "" {
		root = "."
	}

	// Create a temporary directory in the current working directory
	temp, err := os.MkdirTemp(".", tempPrefix+"-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	tempDir, absErr := filepath.Abs(temp)
	if absErr != nil {
		// Fallback to relative path if Abs fails
		tempDir = temp
	}

	// Idempotent cleanup
	var once sync.Once
	cleanup := func() {
		once.Do(func() { _ = os.RemoveAll(tempDir) })
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

		dst := filepath.Join(tempDir, rel)
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

	return tempDir, cleanup, nil
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
