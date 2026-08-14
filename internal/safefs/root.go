// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

// Package safefs provides root-relative filesystem access for paths obtained
// from a repository or workspace. A Root keeps every operation below one
// directory, including when a path contains a symlink.
package safefs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Root is a filesystem view rooted at one directory.
//
// The underlying os.Root rejects absolute paths and paths that escape the
// directory. Keeping that capability behind this small package makes the
// repository boundary explicit at call sites and gives all readers one common
// implementation.
type Root struct {
	root *os.Root
}

// OpenRoot opens path as the root of a repository-local filesystem view.
func OpenRoot(path string) (*Root, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open filesystem root %q: %w", path, err)
	}

	return &Root{root: root}, nil
}

// Close releases the resources held by the root.
func (r *Root) Close() error {
	if r == nil || r.root == nil {
		return nil
	}

	return r.root.Close()
}

// Open opens a file or directory below the root.
func (r *Root) Open(name string) (*os.File, error) {
	name, err := relative(name)
	if err != nil {
		return nil, err
	}

	return r.root.Open(name)
}

// OpenRoot opens a child directory below the root and returns a filesystem
// view anchored to that directory. The child is resolved by the parent
// os.Root, so later path changes cannot turn it into a string-based escape.
func (r *Root) OpenRoot(name string) (*Root, error) {
	name, err := relative(name)
	if err != nil {
		return nil, err
	}

	root, err := r.root.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("open child filesystem root %q: %w", name, err)
	}

	return &Root{root: root}, nil
}

// ReadFile reads a regular file below the root.
func (r *Root) ReadFile(name string) ([]byte, error) {
	name, err := relative(name)
	if err != nil {
		return nil, err
	}

	return r.root.ReadFile(name)
}

// ReadDir reads directory entries below the root.
func (r *Root) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	return file.ReadDir(-1)
}

// Stat follows a path below the root and returns its metadata. A symlink that
// points outside the root is rejected by os.Root rather than followed.
func (r *Root) Stat(name string) (os.FileInfo, error) {
	name, err := relative(name)
	if err != nil {
		return nil, err
	}

	return r.root.Stat(name)
}

// Lstat returns metadata for the path itself, without following its final
// symlink. This is used when a repository wants to render a link target as
// text instead of reading the target's contents.
func (r *Root) Lstat(name string) (os.FileInfo, error) {
	name, err := relative(name)
	if err != nil {
		return nil, err
	}

	return r.root.Lstat(name)
}

// Readlink reads the link target string for a path below the root. The target
// is returned as data; it is not followed by this operation.
func (r *Root) Readlink(name string) (string, error) {
	name, err := relative(name)
	if err != nil {
		return "", err
	}

	return r.root.Readlink(name)
}

// relative validates a path supplied to a root-relative operation. os.Root
// performs the same boundary check, but rejecting malformed paths here keeps
// the contract stable if callers are ever changed to a different backend.
func relative(name string) (string, error) {
	if name == "" {
		return ".", nil
	}

	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes filesystem root", name)
	}

	return clean, nil
}
