// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloneOrUpdateResetUsesPreparedExactBranchWithoutSecondFetch(t *testing.T) {
	work, develop := preparedResetFixture(t)

	result, err := NewClient().CloneOrUpdate(context.Background(), CloneOrUpdateOptions{
		URL:                 "https://example.invalid/repo.git",
		Destination:         work,
		Strategy:            StrategyReset,
		Branch:              "develop",
		ExactBranchPrepared: true,
	})
	if err != nil {
		t.Fatalf("CloneOrUpdate() error = %v, want prepared reset to avoid unreachable origin", err)
	}
	if result.Action != "reset" {
		t.Errorf("Action = %q, want reset", result.Action)
	}
	if got := gitOutput(t, work, "rev-parse", "HEAD"); got != develop {
		t.Errorf("HEAD = %s, want prepared origin/develop %s", got, develop)
	}
}

func TestCloneOrUpdateResetFetchesWhenExactBranchIsNotPrepared(t *testing.T) {
	work, _ := preparedResetFixture(t)

	_, err := NewClient().CloneOrUpdate(context.Background(), CloneOrUpdateOptions{
		URL:         "https://example.invalid/repo.git",
		Destination: work,
		Strategy:    StrategyReset,
		Branch:      "develop",
	})
	if err == nil || !strings.Contains(err.Error(), "fetch before reset failed") {
		t.Fatalf("CloneOrUpdate() error = %v, want normal reset fetch failure", err)
	}
}

func preparedResetFixture(t *testing.T) (work, develop string) {
	t.Helper()
	work = initTestGitRepo(t, t.TempDir())
	runGitCmd(t, work, "checkout", "-b", "develop")
	if err := os.WriteFile(filepath.Join(work, "develop.txt"), []byte("develop\n"), 0o600); err != nil {
		t.Fatalf("write develop: %v", err)
	}
	runGitCmd(t, work, "add", "develop.txt")
	runGitCmd(t, work, "commit", "-m", "develop")
	develop = gitOutput(t, work, "rev-parse", "HEAD")
	runGitCmd(t, work, "checkout", "-")
	runGitCmd(t, work, "remote", "add", "origin", filepath.Join(t.TempDir(), "unreachable.git"))
	runGitCmd(t, work, "update-ref", "refs/remotes/origin/develop", develop)
	return work, develop
}
