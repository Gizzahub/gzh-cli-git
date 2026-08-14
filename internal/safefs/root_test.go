// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package safefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootRejectsPathsOutsideRoot(t *testing.T) {
	base := t.TempDir()
	rootPath := filepath.Join(base, "root")
	if err := os.Mkdir(rootPath, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	root, err := OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	for _, name := range []string{"../outside.txt", outside} {
		if _, err := root.ReadFile(name); err == nil {
			t.Errorf("ReadFile(%q) succeeded, want root escape error", name)
		}
	}
}

func TestRootDoesNotFollowSymlinkOutsideRoot(t *testing.T) {
	base := t.TempDir()
	rootPath := filepath.Join(base, "root")
	if err := os.Mkdir(rootPath, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside.txt")
	secret := "AWS_SECRET_ACCESS_KEY=must-not-be-read\n"
	if err := os.WriteFile(outside, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(rootPath, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	root, err := OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	if _, err := root.ReadFile("link.txt"); err == nil {
		t.Fatal("ReadFile(link.txt) succeeded, want symlink escape error")
	}
	if target, err := root.Readlink("link.txt"); err != nil || !strings.Contains(target, "outside.txt") {
		t.Errorf("Readlink(link.txt) = %q, %v; want link target text", target, err)
	}
}
