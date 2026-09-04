// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package safefs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootOpenFileRejectsEscape(t *testing.T) {
	base := t.TempDir()
	rootPath := filepath.Join(base, "root")
	if err := os.Mkdir(rootPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "ok.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile("ok.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := root.OpenFile("../ok.txt", os.O_RDONLY, 0); err == nil {
		t.Fatal("OpenFile escape succeeded")
	}
}

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

func TestRootOpenRootRejectsEscapeAndAnchorsChild(t *testing.T) {
	base := t.TempDir()
	rootPath := filepath.Join(base, "root")
	childPath := filepath.Join(rootPath, "repo")
	if err := os.MkdirAll(childPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childPath, "marker"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	root, err := OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	for _, name := range []string{"../outside", filepath.Join(base, "outside")} {
		if _, err := root.OpenRoot(name); err == nil {
			t.Errorf("OpenRoot(%q) succeeded, want boundary error", name)
		}
	}

	child, err := root.OpenRoot("repo")
	if err != nil {
		t.Fatalf("OpenRoot(repo): %v", err)
	}
	defer func() { _ = child.Close() }()

	movedPath := filepath.Join(rootPath, "repo-moved")
	if err := os.Rename(childPath, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(childPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childPath, "marker"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := child.ReadFile("marker")
	if err != nil {
		t.Fatalf("child.ReadFile(marker): %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("child root followed renamed path and read %q, want original", data)
	}
}
