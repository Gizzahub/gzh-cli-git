// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package contextref

import (
	"context"
	"crypto/sha1" //nolint:gosec // G505: Git object IDs, not a security digest
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

func gitWorktreeRoot(ctx context.Context, git *gitcmd.Executor, dir string) (string, error) {
	out, err := git.RunOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errNotGit
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", errNotGit
	}
	return root, nil
}

func gitObjectFormat(ctx context.Context, git *gitcmd.Executor, dir string) string {
	out, err := git.RunOutput(ctx, dir, "rev-parse", "--show-object-format")
	if err != nil {
		return "sha1"
	}
	algo := strings.TrimSpace(out)
	if algo == "" {
		return "sha1"
	}
	return algo
}

func gitIndexBlob(ctx context.Context, git *gitcmd.Executor, dir, path string) (gitBlob, bool, error) {
	out, err := git.RunOutput(ctx, dir, "ls-files", "--stage", "--", literalPathspec(path))
	if err != nil {
		return gitBlob{}, false, err
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return gitBlob{}, false, nil
	}
	lines := strings.Split(line, "\n")
	if len(lines) != 1 {
		return gitBlob{}, false, fmt.Errorf("pathspec matched %d index entries", len(lines))
	}
	blob, ok := parseLsFiles(lines[0])
	return blob, ok, nil
}

func gitHEADBlob(ctx context.Context, git *gitcmd.Executor, dir, path string) (gitBlob, error) {
	out, err := git.RunOutput(ctx, dir, "ls-tree", "HEAD", "--", literalPathspec(path))
	if err != nil {
		return gitBlob{}, err
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return gitBlob{}, nil
	}
	lines := strings.Split(line, "\n")
	if len(lines) != 1 {
		return gitBlob{}, fmt.Errorf("pathspec matched %d HEAD entries", len(lines))
	}
	blob, ok := parseLsTree(lines[0])
	if !ok {
		return gitBlob{}, nil
	}
	return blob, nil
}

func literalPathspec(path string) string {
	return ":(literal)" + path
}

func parseLsFiles(line string) (gitBlob, bool) {
	mode, rest, ok := strings.Cut(line, " ")
	if !ok {
		return gitBlob{}, false
	}
	oid, rest, ok := strings.Cut(rest, " ")
	if !ok {
		return gitBlob{}, false
	}
	stage, _, ok := strings.Cut(rest, "\t")
	if !ok || stage != "0" {
		return gitBlob{}, false
	}
	return gitBlob{Mode: mode, OID: oid}, true
}

func parseLsTree(line string) (gitBlob, bool) {
	mode, rest, ok := strings.Cut(line, " ")
	if !ok {
		return gitBlob{}, false
	}
	kind, rest, ok := strings.Cut(rest, " ")
	if !ok {
		return gitBlob{}, false
	}
	oid, _, ok := strings.Cut(rest, "\t")
	if !ok {
		return gitBlob{}, false
	}
	_ = kind
	return gitBlob{Mode: mode, OID: oid}, true
}

func qualifyOID(algo, oid string) string {
	if oid == "" {
		return ""
	}
	if strings.Contains(oid, ":") {
		return oid
	}
	if algo == "" {
		algo = "sha1"
	}
	return algo + ":" + oid
}

func gitBlobDigest(algo string, data []byte) string {
	var h hash.Hash
	if algo == "sha256" {
		h = sha256.New()
	} else {
		h = sha1.New() //nolint:gosec // G401: Git SHA-1 object ID
		algo = "sha1"
	}
	_, _ = fmt.Fprintf(h, "blob %d\x00", len(data))
	_, _ = h.Write(data)
	return algo + ":" + hex.EncodeToString(h.Sum(nil))
}

func contentDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func regularGitMode(mode string) bool {
	return mode == "100644" || mode == "100755"
}
