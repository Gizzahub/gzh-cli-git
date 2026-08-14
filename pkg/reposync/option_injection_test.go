// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"context"
	"strings"
	"testing"
)

// TestCheckoutBranch_RejectsOptionInjection verifies that checkoutBranch — which
// bypasses pkg/repository and calls git directly — rejects config-derived branch
// names git could read as options, before any git process runs (AC4).
func TestCheckoutBranch_RejectsOptionInjection(t *testing.T) {
	ctx := context.Background()
	logger := nopGitLogger{}
	dir := t.TempDir() // not a git repo; rejection must precede any git op

	for _, br := range []string{"--upload-pack=/tmp/evil", "-x", "--output=/tmp/x"} {
		_, err := checkoutBranch(ctx, dir, br, logger)
		if err == nil || !strings.Contains(err.Error(), "invalid branch name") {
			t.Fatalf("branch %q: expected invalid branch name error, got %v", br, err)
		}
	}
}

// TestCheckoutBranch_AllowsValidBranch guards AC3: a legitimate comma-separated
// fallback list must pass the validator. On a non-repo dir it fails later with a
// "none of the specified branches exist" error, never "invalid branch name".
func TestCheckoutBranch_AllowsValidBranch(t *testing.T) {
	ctx := context.Background()
	logger := nopGitLogger{}
	dir := t.TempDir()

	_, err := checkoutBranch(ctx, dir, "develop,master", logger)
	if err != nil && strings.Contains(err.Error(), "invalid branch name") {
		t.Fatalf("valid branch list wrongly rejected: %v", err)
	}
}

func TestCheckoutBranch_PreservesHEAD(t *testing.T) {
	msg, err := checkoutBranch(context.Background(), t.TempDir(), "HEAD", nopGitLogger{})
	if err != nil {
		t.Fatalf("checkoutBranch(HEAD) error = %v", err)
	}
	if msg != "kept current HEAD" {
		t.Fatalf("checkoutBranch(HEAD) message = %q, want current HEAD preserved", msg)
	}
}

func TestAddAdditionalRemotes_RejectsInvalidInputsBeforeGit(t *testing.T) {
	ctx := context.Background()
	logger := nopGitLogger{}
	dir := t.TempDir() // validation must happen before this path is used by git

	for name, remoteURL := range map[string]string{
		"--upload-pack=/tmp/evil": "https://example.com/repo.git",
		"backup":                  "https://example.com/a;touch /tmp/pwned",
	} {
		if _, err := addAdditionalRemotes(ctx, dir, map[string]string{name: remoteURL}, logger); err == nil {
			t.Fatalf("remote %q URL %q: expected validation error", name, remoteURL)
		}
	}
}
