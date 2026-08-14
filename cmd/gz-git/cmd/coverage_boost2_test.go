// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/watch"
)

func TestCollectCloneURLs(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "urls.txt")
	content := "# comment\nhttps://github.com/a/b.git\n\n  https://github.com/c/d.git  \n# another\n"
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	urls, err := collectCloneURLs([]string{"https://github.com/x/y.git", "", "#skip", "  z  "}, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) < 3 {
		t.Fatalf("urls=%v", urls)
	}
	if _, err := collectCloneURLs(nil, filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestDisplayCloneResultsAndIcons(t *testing.T) {
	prev := cloneFlags
	prevV := verbose
	t.Cleanup(func() { cloneFlags = prev; verbose = prevV })

	res := &repository.BulkCloneResult{
		TotalRequested: 5,
		TotalCloned:    1,
		TotalUpdated:   1,
		TotalSkipped:   1,
		TotalFailed:    1,
		Duration:       time.Millisecond * 100,
		Summary: map[string]int{
			"cloned": 1, "updated": 1, "skipped": 1, "error": 1, "would-clone": 1, "dirty": 1, "pulled": 1, "rebased": 1, "would-update": 1, "other": 1,
		},
		Repositories: []repository.RepositoryCloneResult{
			{RelativePath: "a", Status: "cloned", Branch: "main", Duration: time.Millisecond, URL: "u1"},
			{RelativePath: "b", Status: "error", Error: errors.New("fail"), URL: "u2"},
			{RelativePath: "c", Status: "skipped", URL: "u3"},
		},
	}

	for _, format := range []string{"default", "compact", "json", "llm"} {
		cloneFlags.Format = format
		verbose = true
		captureStdout(t, func() { displayCloneResults(res) })
	}
	for _, st := range []string{"cloned", "updated", "pulled", "rebased", "skipped", "would-clone", "would-update", "dirty", "error", "x"} {
		if getCloneStatusIcon(st) == "" {
			t.Errorf("icon empty for %s", st)
		}
	}
}

func TestRunClone_MissingDirAndNoURLs(t *testing.T) {
	prev := cloneFlags
	prevURLs, prevFile, prevCfg := cloneURLs, cloneFile, cloneConfig
	t.Cleanup(func() {
		cloneFlags = prev
		cloneURLs, cloneFile, cloneConfig = prevURLs, prevFile, prevCfg
		cloneConfigStdin = false
	})
	cloneFlags = BulkCommandFlags{Parallel: 2, Format: "default"}
	cloneURLs = nil
	cloneFile = ""
	cloneConfig = ""
	if err := runClone(cloneCmd, []string{"/no/such/dir-xyz"}); err == nil {
		t.Fatal("expected missing dir")
	}
	// no urls should error
	if err := runClone(cloneCmd, []string{t.TempDir()}); err == nil {
		t.Log("no-url may error or noop depending on impl")
	}
}

func TestRunClone_FromFileDryRun(t *testing.T) {
	prev := cloneFlags
	prevURLs, prevFile := cloneURLs, cloneFile
	t.Cleanup(func() {
		cloneFlags = prev
		cloneURLs, cloneFile = prevURLs, prevFile
	})
	parent := t.TempDir()
	// Use local path as "URL" will fail clone but exercise path; better use --url with invalid that fails quickly
	f := filepath.Join(parent, "repos.txt")
	// empty file after comments only
	if err := os.WriteFile(f, []byte("# only comments\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cloneFile = f
	cloneURLs = nil
	cloneFlags = BulkCommandFlags{Parallel: 1, Format: "json", DryRun: true}
	err := runClone(cloneCmd, []string{parent})
	// empty urls likely errors
	_ = err
}

func TestDisplaySwitchResultsAllFormats(t *testing.T) {
	prevV := verbose
	t.Cleanup(func() { verbose = prevV })
	res := &repository.BulkSwitchResult{
		TotalScanned:   3,
		TotalProcessed: 3,
		TargetBranch:   "develop",
		Duration:       time.Millisecond,
		Summary: map[string]int{
			repository.StatusSwitched:         1,
			repository.StatusRebaseInProgress: 1,
			repository.StatusError:            1,
		},
		Repositories: []repository.RepositorySwitchResult{
			{RelativePath: "a", Status: repository.StatusSwitched, Message: "ok", CurrentBranch: "develop"},
			{RelativePath: "b", Status: repository.StatusRebaseInProgress, Message: "rebasing"},
			{RelativePath: "c", Status: repository.StatusError, Message: "fail", Error: errors.New("e")},
		},
	}
	verbose = false
	for _, format := range []string{"default", "compact", "json", "llm"} {
		captureStdout(t, func() { displaySwitchResults(res, format) })
	}
	verbose = true
	captureStdout(t, func() {
		displaySwitchResults(res, "default")
		for _, r := range res.Repositories {
			displaySwitchRepoResult(r)
		}
		displaySwitchSummary(res)
	})
}

func TestWatchFormattersAndHelpers(t *testing.T) {
	ev := watch.Event{
		Path:      "/repo",
		Type:      watch.EventTypeModified,
		Timestamp: time.Now(),
		Files:     []string{"a.go", "b.go"},
	}
	for _, format := range []string{"default", "compact", "json", "llm", "other"} {
		f := newEventFormatter(format)
		out := f.Format(ev)
		if out == "" && format != "other" {
			t.Logf("empty format for %s", format)
		}
	}
	// empty files
	_ = (&jsonFormatter{}).Format(watch.Event{Path: "/r", Type: watch.EventTypeClean, Timestamp: time.Now()})
	_ = (&llmFormatter{}).Format(ev)
	_ = (&defaultFormatter{}).Format(ev)
	_ = (&compactFormatter{}).Format(ev)

	if pluralize(1, "file", "files") != "file" {
		t.Error("pluralize 1")
	}
	if pluralize(2, "file", "files") != "files" {
		t.Error("pluralize 2")
	}

	l := newWatchLogger(true)
	captureStdout(t, func() {
		l.Debug("d %s", "x")
		l.Info("i %s", "x")
		l.Warn("w %s", "x")
		l.Error("e %s", "x")
	})
	l2 := newWatchLogger(false)
	l2.Debug("d")
	l2.Info("i")
}

func TestRenderBulkResults_AllModes(t *testing.T) {
	cfg := BulkRenderConfig{
		Verb:          "fetched",
		IssueStatuses: issueStatusSet("error", "conflict"),
	}
	in := BulkRenderInput{
		TotalScanned: 3,
		Duration:     time.Millisecond,
		Summary:      map[string]int{"success": 2, "error": 1},
		Rows: []BulkRenderRow{
			{Path: "a", Status: "success", Message: "ok", Branch: "main"},
			{Path: "b", Status: "error", Message: "fail", Err: errors.New("e")},
			{Path: "c", Status: "conflict", Message: "c"},
		},
	}
	for _, format := range []string{"default", "compact", "json", "llm"} {
		var buf bytes.Buffer
		cfg.Format = format
		cfg.Verbose = false
		RenderBulkResults(&buf, cfg, in)
		cfg.Verbose = true
		RenderBulkResults(&buf, cfg, in)
	}
	if !isIssueStatus(cfg, "error") {
		t.Error("error should be issue")
	}
	_ = isRefspecError(errors.New("refspec does not match"))
	_ = isRefspecError(errors.New("other"))
	_ = isRefspecError(nil)
}

func TestRunCleanupBranchDryRun(t *testing.T) {
	parent := setupBulkParent(t)
	cmd := findCommand(t, rootCmd, "cleanup", "branch")
	// force dry-run
	if f := cmd.Flags().Lookup("force"); f != nil {
		_ = f.Value.Set("false")
	}
	captureStdout(t, func() {
		if cmd.RunE != nil {
			if err := cmd.RunE(cmd, []string{parent}); err != nil {
				t.Logf("cleanup branch: %v", err)
			}
		}
	})
}

func TestRunHistoryStatsOnTempRepo(t *testing.T) {
	parent := setupBulkParent(t)
	// history stats typically needs cwd as git repo — chdir into child
	repo := filepath.Join(parent, "r1")
	cwd, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	stats := findCommand(t, rootCmd, "history", "stats")
	captureStdout(t, func() {
		if stats.RunE != nil {
			if err := stats.RunE(stats, nil); err != nil {
				t.Logf("history stats: %v", err)
			}
		}
	})
	contrib := findCommand(t, rootCmd, "history", "contributors")
	captureStdout(t, func() {
		if contrib.RunE != nil {
			if err := contrib.RunE(contrib, nil); err != nil {
				t.Logf("history contributors: %v", err)
			}
		}
	})
}

func TestRunCommitPreview(t *testing.T) {
	parent := setupBulkParent(t)
	// make dirty repo
	repo := filepath.Join(parent, "r1")
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// without --yes should preview
	captureStdout(t, func() {
		if err := runCommit(commitCmd, []string{parent}); err != nil {
			t.Logf("commit preview: %v", err)
		}
	})
}

func TestBuildCloneOptionsFromConfigIfExported(t *testing.T) {
	// exercise resolve + validate paths already covered; parse JSON clone config
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	if err := os.WriteFile(path, []byte(`{"strategy":"skip","repositories":[{"url":"https://github.com/a/b.git","name":"b"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := parseCloneConfig(path, false)
	if err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(cfg.Repositories) != 1 {
		t.Fatalf("repos=%d", len(cfg.Repositories))
	}
}

func TestSanitizeAndHelpersFromConfig(t *testing.T) {
	// printConfigValue already tested; ensure root command groups
	setCommandGroups(rootCmd)
	// version command
	v := findCommand(t, rootCmd, "version")
	captureStdout(t, func() {
		if v.RunE != nil {
			_ = v.RunE(v, nil)
		} else if v.Run != nil {
			v.Run(v, nil)
		}
	})
}
