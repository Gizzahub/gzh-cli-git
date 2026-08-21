// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

func TestCleanupService_AnalyzeBotsOnlyRemoteMerged(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, dir, "branch", "-M", "master")

	gitCommit(t, dir, "checkout", "-b", "tmp-merged")
	writeAndCommit(t, dir, "landed.txt", "landed")
	mergedSHA := gitOutput(t, dir, "rev-parse", "HEAD")

	gitCommit(t, dir, "checkout", "-b", "tmp-open", "master")
	writeAndCommit(t, dir, "open.txt", "open")
	openSHA := gitOutput(t, dir, "rev-parse", "HEAD")

	gitCommit(t, dir, "checkout", "master")
	gitCommit(t, dir, "merge", "--no-ff", "--no-edit", "tmp-merged")
	gitCommit(t, dir, "branch", "-D", "tmp-merged")
	gitCommit(t, dir, "branch", "-D", "tmp-open")

	masterSHA := gitOutput(t, dir, "rev-parse", "HEAD")
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/dependabot/go_modules/x", mergedSHA)
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/dependabot/go_modules/unmerged", openSHA)
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/develop", masterSHA)
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/feat/done", mergedSHA)

	repo := &repository.Repository{Path: dir}
	report, err := NewCleanupService().Analyze(context.Background(), repo, AnalyzeOptions{
		IncludeMerged: true,
		IncludeRemote: true,
		BotsOnly:      true,
		BaseBranch:    "master",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(report.Merged) != 1 || report.Merged[0].Name != "dependabot/go_modules/x" {
		t.Fatalf("Merged = %+v, want [dependabot/go_modules/x]", namesOf(report.Merged))
	}
	if !report.Merged[0].IsRemote {
		t.Error("merged bot should be marked remote-only")
	}
	if report.Merged[0].Name == "origin/dependabot/go_modules/x" {
		t.Error("stored name still has origin/ prefix")
	}

	for _, b := range report.Merged {
		if b.Name == "dependabot/go_modules/unmerged" {
			t.Error("unmerged bot classified as merged")
		}
		if b.Name == "develop" || b.Name == "feat/done" {
			t.Errorf("BotsOnly leaked %q", b.Name)
		}
	}
}

func TestCleanupService_ExecuteDeletesRemoteOnly(t *testing.T) {
	seed := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, seed, "branch", "-M", "master")
	gitCommit(t, seed, "checkout", "-b", "dependabot/go_modules/x")
	writeAndCommit(t, seed, "bot.txt", "bot")
	gitCommit(t, seed, "checkout", "master")
	gitCommit(t, seed, "merge", "--no-ff", "--no-edit", "dependabot/go_modules/x")

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	clone := filepath.Join(root, "clone")
	gitCommit(t, t.TempDir(), "clone", "--bare", seed, origin)
	gitCommit(t, t.TempDir(), "clone", origin, clone)

	repo := &repository.Repository{Path: clone}
	svc := NewCleanupService()
	report, err := svc.Analyze(context.Background(), repo, AnalyzeOptions{
		IncludeMerged: true,
		IncludeRemote: true,
		BotsOnly:      true,
		BaseBranch:    "master",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.IsEmpty() {
		t.Fatal("expected remote-only merged bot in report")
	}

	result, err := svc.Execute(context.Background(), repo, report, ExecuteOptions{
		Force:  true,
		Remote: true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Failed) != 0 {
		t.Fatalf("Failed = %+v", result.Failed)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != "dependabot/go_modules/x" {
		t.Errorf("Deleted = %v, want [dependabot/go_modules/x]", result.Deleted)
	}

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/dependabot/go_modules/x") //nolint:noctx // test helper
	cmd.Dir = origin
	if cmd.Run() == nil {
		t.Error("origin still has dependabot/go_modules/x")
	}
}

func TestCleanupService_ExecuteReportsPushDeleteFailure(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, dir, "branch", "-M", "master")
	gitCommit(t, dir, "checkout", "-b", "tmp-merged")
	writeAndCommit(t, dir, "landed.txt", "landed")
	mergedSHA := gitOutput(t, dir, "rev-parse", "HEAD")
	gitCommit(t, dir, "checkout", "master")
	gitCommit(t, dir, "merge", "--no-ff", "--no-edit", "tmp-merged")
	gitCommit(t, dir, "branch", "-D", "tmp-merged")
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/dependabot/go_modules/x", mergedSHA)
	gitCommit(t, dir, "remote", "add", "origin", filepath.Join(dir, "no-such-origin.git"))

	repo := &repository.Repository{Path: dir}
	svc := NewCleanupService()
	report, err := svc.Analyze(context.Background(), repo, AnalyzeOptions{
		IncludeMerged: true,
		IncludeRemote: true,
		BotsOnly:      true,
		BaseBranch:    "master",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if report.IsEmpty() {
		t.Fatal("expected remote-only merged bot in report")
	}

	result, err := svc.Execute(context.Background(), repo, report, ExecuteOptions{Force: true, Remote: true})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("Deleted = %v, want none on push failure", result.Deleted)
	}
	if len(result.Failed) == 0 {
		t.Fatal("push --delete failure was reported as success")
	}
}

func TestCleanupService_ExecuteLeaseRefusesMovedRemoteTip(t *testing.T) {
	seed := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, seed, "branch", "-M", "master")
	gitCommit(t, seed, "checkout", "-b", "dependabot/go_modules/x")
	writeAndCommit(t, seed, "bot.txt", "bot")
	gitCommit(t, seed, "checkout", "master")
	gitCommit(t, seed, "merge", "--no-ff", "--no-edit", "dependabot/go_modules/x")

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	clone := filepath.Join(root, "clone")
	gitCommit(t, t.TempDir(), "clone", "--bare", seed, origin)
	gitCommit(t, t.TempDir(), "clone", origin, clone)

	repo := &repository.Repository{Path: clone}
	svc := NewCleanupService()
	report, err := svc.Analyze(context.Background(), repo, AnalyzeOptions{
		IncludeMerged: true,
		IncludeRemote: true,
		BotsOnly:      true,
		BaseBranch:    "master",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(report.Merged) != 1 || report.Merged[0].Name != "dependabot/go_modules/x" {
		t.Fatalf("Merged = %+v, want [dependabot/go_modules/x]", namesOf(report.Merged))
	}
	classifiedSHA := gitOutput(t, clone, "rev-parse", "refs/remotes/origin/dependabot/go_modules/x")
	if report.Merged[0].SHA != classifiedSHA {
		t.Fatalf("classified SHA = %q, want full tracking SHA %q", report.Merged[0].SHA, classifiedSHA)
	}

	other := filepath.Join(t.TempDir(), "other")
	gitCommit(t, t.TempDir(), "clone", origin, other)
	gitCommit(t, other, "config", "user.email", "test@test.com")
	gitCommit(t, other, "config", "user.name", "Test")
	gitCommit(t, other, "config", "commit.gpgsign", "false")
	gitCommit(t, other, "checkout", "-B", "dependabot/go_modules/x", "origin/dependabot/go_modules/x")
	writeAndCommit(t, other, "sneak.txt", "sneak")
	gitCommit(t, other, "push", "origin", "dependabot/go_modules/x")
	newSHA := gitOutput(t, other, "rev-parse", "HEAD")

	result, err := svc.Execute(context.Background(), repo, report, ExecuteOptions{
		Force:  true,
		Remote: true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Deleted) != 0 {
		t.Errorf("Deleted = %v, want none; lease must refuse a moved tip", result.Deleted)
	}
	if len(result.Failed) == 0 {
		t.Fatal("moved remote tip was reported as success")
	}

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/dependabot/go_modules/x") //nolint:noctx // test helper
	cmd.Dir = origin
	if err := cmd.Run(); err != nil {
		t.Fatal("moved remote branch must still exist")
	}
	got := gitOutput(t, origin, "rev-parse", "refs/heads/dependabot/go_modules/x")
	if got != newSHA {
		t.Errorf("origin tip = %s, want new commit %s", got, newSHA)
	}
}

func TestCleanupService_ExecuteDoesNotDeleteUnmergedRemote(t *testing.T) {
	seed := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, seed, "branch", "-M", "master")
	gitCommit(t, seed, "checkout", "-b", "dependabot/go_modules/x")
	writeAndCommit(t, seed, "bot.txt", "bot")
	mergedSHA := gitOutput(t, seed, "rev-parse", "HEAD")
	gitCommit(t, seed, "checkout", "master")
	gitCommit(t, seed, "merge", "--no-ff", "--no-edit", "dependabot/go_modules/x")

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	clone := filepath.Join(root, "clone")
	gitCommit(t, t.TempDir(), "clone", "--bare", seed, origin)
	gitCommit(t, t.TempDir(), "clone", origin, clone)

	gitCommit(t, clone, "checkout", "dependabot/go_modules/x")
	writeAndCommit(t, clone, "open.txt", "open-pr")
	gitCommit(t, clone, "push", "origin", "dependabot/go_modules/x")
	gitCommit(t, clone, "checkout", "master")
	gitCommit(t, clone, "branch", "-f", "dependabot/go_modules/x", mergedSHA)
	gitCommit(t, clone, "fetch", "origin")

	repo := &repository.Repository{Path: clone}
	svc := NewCleanupService()
	report, err := svc.Analyze(context.Background(), repo, AnalyzeOptions{
		IncludeMerged: true,
		IncludeRemote: true,
		BotsOnly:      true,
		BaseBranch:    "master",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if _, err := svc.Execute(context.Background(), repo, report, ExecuteOptions{Force: true, Remote: true}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/dependabot/go_modules/x") //nolint:noctx // test helper
	cmd.Dir = origin
	if err := cmd.Run(); err != nil {
		t.Fatal("origin lost unmerged dependabot/go_modules/x")
	}
}

func TestCleanupService_AnalyzeBotsOnlyRemoteSuperseded(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, dir, "branch", "-M", "master")
	writeAndCommit(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n\nrequire github.com/aws/aws-sdk-go-v2 v1.32.0\n")

	gitCommit(t, dir, "checkout", "-b", "tmp-bot")
	writeAndCommit(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n\nrequire github.com/aws/aws-sdk-go-v2 v1.40.0\n")
	botSHA := gitOutput(t, dir, "rev-parse", "HEAD")

	gitCommit(t, dir, "checkout", "master")
	writeAndCommit(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n\nrequire github.com/aws/aws-sdk-go-v2 v1.41.1\n")
	gitCommit(t, dir, "branch", "-D", "tmp-bot")
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/dependabot/go_modules/github.com/aws/aws-sdk-go-v2-1.40.0", botSHA)
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/feat/human", botSHA)

	repo := &repository.Repository{Path: dir}
	report, err := NewCleanupService().Analyze(context.Background(), repo, AnalyzeOptions{
		IncludeSuperseded: true,
		IncludeRemote:     true,
		BotsOnly:          true,
		BaseBranch:        "master",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(report.Merged) != 0 {
		t.Errorf("Merged = %v, want none", namesOf(report.Merged))
	}
	if len(report.Superseded) != 1 || report.Superseded[0].Name != "dependabot/go_modules/github.com/aws/aws-sdk-go-v2-1.40.0" {
		t.Fatalf("Superseded = %v, want the unmerged bot whose version already landed", namesOf(report.Superseded))
	}
	if !report.Superseded[0].IsRemote {
		t.Error("superseded bot should be remote")
	}
	if report.Superseded[0].SHA == "" {
		t.Error("superseded bot is missing the classified SHA for a lease")
	}
	for _, b := range report.GetAllBranches() {
		if b.Name == "feat/human" {
			t.Error("--superseded leaked a human topic branch")
		}
	}
}

func TestCleanupService_AnalyzeStillNewerBotNotSuperseded(t *testing.T) {
	dir := testutil.TempGitRepoWithCommit(t)
	gitCommit(t, dir, "branch", "-M", "master")
	writeAndCommit(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n\nrequire github.com/aws/aws-sdk-go-v2 v1.32.0\n")

	gitCommit(t, dir, "checkout", "-b", "tmp-bot")
	writeAndCommit(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n\nrequire github.com/aws/aws-sdk-go-v2 v1.41.1\n")
	botSHA := gitOutput(t, dir, "rev-parse", "HEAD")
	gitCommit(t, dir, "checkout", "master")
	gitCommit(t, dir, "branch", "-D", "tmp-bot")
	gitCommit(t, dir, "update-ref", "refs/remotes/origin/dependabot/go_modules/github.com/aws/aws-sdk-go-v2-1.41.1", botSHA)

	repo := &repository.Repository{Path: dir}
	report, err := NewCleanupService().Analyze(context.Background(), repo, AnalyzeOptions{
		IncludeSuperseded: true,
		IncludeRemote:     true,
		BotsOnly:          true,
		BaseBranch:        "master",
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(report.Superseded) != 0 {
		t.Errorf("Superseded = %v, want none for a still-newer bot", namesOf(report.Superseded))
	}
}

func writeAndCommit(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	gitCommit(t, dir, "add", name)
	gitCommit(t, dir, "commit", "-m", "add "+name)
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:noctx // test helper
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func namesOf(branches []*Branch) []string {
	out := make([]string, 0, len(branches))
	for _, b := range branches {
		out = append(out, b.Name)
	}
	return out
}
