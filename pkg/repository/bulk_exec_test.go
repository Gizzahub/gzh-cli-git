// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func setupExecTree(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	repo := filepath.Join(parent, "r1")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	cmd := exec.CommandContext(t.Context(), "git", "init", "r1")
	cmd.Dir = parent
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	run("config", "user.email", "t@t.com")
	run("config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "i")
	return parent
}

func TestBulkExec_SuccessAndEnv(t *testing.T) {
	parent := setupExecTree(t)
	client := repository.NewClient()
	// printenv reports one of the variables BulkExec injects, which is the point
	// of the test. It is also a program that reads no stdin and exits on its own:
	// running a bare shell here would block on an empty stdin instead.
	result, err := client.BulkExec(context.Background(), repository.BulkExecOptions{
		Directory: parent,
		MaxDepth:  1,
		Parallel:  2,
		Command:   "printenv",
		Args:      []string{"GZ_REPO_NAME"},
	})
	if err != nil {
		t.Fatalf("BulkExec: %v", err)
	}
	if result.TotalProcessed != 1 {
		t.Fatalf("processed=%d", result.TotalProcessed)
	}
	r := result.Repositories[0]
	if r.Status != repository.StatusExecOK {
		t.Fatalf("status=%s msg=%s out=%s", r.Status, r.Message, r.Output)
	}
	if strings.TrimSpace(r.Output) != "r1" {
		t.Fatalf("GZ_REPO_NAME=%q", r.Output)
	}
}

func TestBulkExec_DryRun(t *testing.T) {
	parent := setupExecTree(t)
	client := repository.NewClient()
	result, err := client.BulkExec(context.Background(), repository.BulkExecOptions{
		Directory: parent,
		MaxDepth:  1,
		DryRun:    true,
		Command:   "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Repositories[0].Status != repository.StatusWouldExec {
		t.Fatalf("status=%s", result.Repositories[0].Status)
	}
}

func TestBulkExec_FailureExitCode(t *testing.T) {
	parent := setupExecTree(t)
	client := repository.NewClient()
	result, err := client.BulkExec(context.Background(), repository.BulkExecOptions{
		Directory: parent,
		MaxDepth:  1,
		Command:   "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	r := result.Repositories[0]
	if r.Status != repository.StatusExecFailed {
		t.Fatalf("status=%s", r.Status)
	}
	if r.ExitCode == 0 {
		t.Fatal("expected non-zero exit")
	}
}

func TestBulkExec_Timeout(t *testing.T) {
	parent := setupExecTree(t)
	client := repository.NewClient()
	result, err := client.BulkExec(context.Background(), repository.BulkExecOptions{
		Directory: parent,
		MaxDepth:  1,
		Command:   "sleep",
		Args:      []string{"5"},
		Timeout:   50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := result.Repositories[0]
	if r.Status != repository.StatusExecFailed {
		t.Fatalf("status=%s msg=%s", r.Status, r.Message)
	}
}

func TestBulkExec_RequiresCommand(t *testing.T) {
	client := repository.NewClient()
	_, err := client.BulkExec(context.Background(), repository.BulkExecOptions{
		Directory: t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
