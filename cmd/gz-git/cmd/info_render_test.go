// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// withColors forces the color gate into a known state for the duration of a
// test, since the result otherwise depends on whether `go test` had a TTY.
func withColors(t *testing.T, on bool) {
	t.Helper()
	was := cliutil.ColorsEnabled()
	t.Cleanup(func() {
		if was {
			cliutil.EnableColors()
		} else {
			cliutil.DisableColors()
		}
	})
	if on {
		cliutil.EnableColors()
	} else {
		cliutil.DisableColors()
	}
}

func bulkResult(repos ...repository.RepositoryStatusResult) *repository.BulkStatusResult {
	return &repository.BulkStatusResult{
		TotalScanned:   len(repos),
		TotalProcessed: len(repos),
		Duration:       120 * time.Millisecond,
		Repositories:   repos,
	}
}

// TestRenderInfoCompact_ColumnsAlignWithColorsOn is the regression guard for
// the bug this layout is built to avoid: padding computed on a string that
// already contains ANSI escapes is short by the length of those escapes, and
// the damage is invisible in a non-TTY test run because the gate blanks them.
// Alignment must therefore be asserted with colors explicitly ON.
func TestRenderInfoCompact_ColumnsAlignWithColorsOn(t *testing.T) {
	withColors(t, true)

	result := bulkResult(
		repository.RepositoryStatusResult{
			Path: "/w/alpha", Branch: "feature/very-long-branch-name",
			Upstream:     "origin/feature/very-long-branch-name",
			CommitsAhead: 3, CommitsBehind: 1,
			StagedFiles: 2, UnstagedFiles: 1, UntrackedFiles: 4,
			RemoteURL: "git@github.com:Acme/alpha.git",
		},
		repository.RepositoryStatusResult{
			Path: "/w/b", Branch: "main", Upstream: "origin/main",
			RemoteURL: "git@github.com:Acme/b.git",
		},
	)
	enr := map[string]infoEnrichment{
		"/w/alpha": {Base: repository.BaseBranchInfo{Name: "main", Source: "config.defaultBranch[0]", Ahead: 3, Behind: 7}, LinkedWorktrees: 2},
		"/w/b":     {Base: repository.BaseBranchInfo{Name: "main", Source: "config.defaultBranch[0]"}},
	}

	var buf bytes.Buffer
	renderInfoTable(&buf, result, enr, false, false)
	out := buf.String()

	if !strings.Contains(out, "\x1b[") {
		t.Fatal("expected ANSI escapes with colors enabled; the test is not exercising the padding bug")
	}

	// Every table line must place the BRANCH column at the same visual offset.
	var tableLines []string
	for _, line := range strings.Split(stripANSI(out), "\n") {
		if strings.Contains(line, "REPOSITORY") || strings.Contains(line, "alpha") || strings.Contains(line, "  b ") {
			tableLines = append(tableLines, line)
		}
	}
	if len(tableLines) < 2 {
		t.Fatalf("expected header plus data lines, got %d:\n%s", len(tableLines), stripANSI(out))
	}

	want := strings.Index(tableLines[0], "BRANCH")
	for _, line := range tableLines[1:] {
		// The repository name column is fixed-width, so the character after it
		// must land on the same offset the header's BRANCH title starts at.
		if len(line) <= want || line[want-1] != ' ' {
			t.Errorf("column misaligned at offset %d in %q", want, line)
		}
	}
}

// cleanRepoTable renders the table for a single repository with nothing at all
// to report, which is the case the two modes disagree about. It returns the
// whole document plus that repository's row on its own: assertions about
// placeholders have to be made against the row, because the summary line above
// it is punctuation-rich and would answer a substring search for either mode.
func cleanRepoTable(t *testing.T, compact bool) (out, row string) {
	t.Helper()

	result := bulkResult(repository.RepositoryStatusResult{
		Path: "/w/clean", Branch: "main", Upstream: "origin/main",
		RemoteURL: "git@github.com:Acme/clean.git",
	})
	enr := map[string]infoEnrichment{
		"/w/clean": {Base: repository.BaseBranchInfo{Name: "main", Source: "heuristic"}},
	}

	var buf bytes.Buffer
	renderInfoTable(&buf, result, enr, false, compact)
	out = buf.String()

	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "clean ") {
			return out, line
		}
	}
	t.Fatalf("no row for the scanned repository:\n%s", out)
	return out, ""
}

// TestRenderInfoTable_KeepsColumnsByDefault is the guard for the complaint that
// produced this mode split: when every repository was clean the table printed
// four columns, and a reader who wanted to know whether anything was behind its
// base could not tell a silent BASE column from a BASE check that never ran.
// The default answers that question — the column is there, and it says so.
func TestRenderInfoTable_KeepsColumnsByDefault(t *testing.T) {
	withColors(t, false)

	out, row := cleanRepoTable(t, false)
	for _, kept := range infoColumns {
		if kept == "BASE" {
			kept = "BASE(" // hoisted to BASE(main) when uniform
		}
		if !strings.Contains(out, kept) {
			t.Errorf("column %q must survive when it has nothing to report:\n%s", kept, out)
		}
	}
	if !strings.Contains(row, cellNormal) {
		t.Errorf("a column with nothing to report must print %q, not blank:\n%s", cellNormal, out)
	}
}

// TestRenderInfoTable_CompactDropsAllEmptyColumns covers the compression rule,
// now behind --compact: a column no repository has anything to say in must not
// occupy its header width.
func TestRenderInfoTable_CompactDropsAllEmptyColumns(t *testing.T) {
	withColors(t, false)

	out, row := cleanRepoTable(t, true)
	for _, dropped := range []string{"UPSTREAM", "DIRTY", "WT"} {
		if strings.Contains(out, dropped) {
			t.Errorf("column %q should be dropped when every cell is empty:\n%s", dropped, out)
		}
	}
	if strings.Contains(out, "BRANCH REMOTE REMOTE ONLY") {
		t.Errorf("REMOTE should be dropped when every REMOTE cell is empty:\n%s", out)
	}
	if !strings.Contains(out, "REPOSITORY") || !strings.Contains(out, "BRANCH") {
		t.Errorf("REPOSITORY and BRANCH must always survive:\n%s", out)
	}
	if strings.Contains(row, cellNormal) {
		t.Errorf("--compact removes empty columns rather than filling them:\n%s", out)
	}
	if strings.Contains(out, "REMOTE ONLY") {
		t.Errorf("REMOTE ONLY should be dropped when it is empty for every repository:\n%s", out)
	}
}

// TestRenderInfoTable_NoRemoteStatedOnce is the table-level guard for the
// duplication the cell tests describe: UPSTREAM and REMOTE both used to print
// "no remote" on the same row, which spends two columns saying one thing and
// invites the reader to look for a second problem that is not there. Filling
// empty cells must not reintroduce it, so both modes are checked.
func TestRenderInfoTable_NoRemoteStatedOnce(t *testing.T) {
	withColors(t, false)

	result := bulkResult(repository.RepositoryStatusResult{
		Path: "/w/local-only", Branch: "master",
		// No Upstream and no RemoteURL: the state a freshly `git init`ed
		// repository is in.
	})
	enr := map[string]infoEnrichment{
		"/w/local-only": {Base: repository.BaseBranchInfo{Name: "master", Source: "heuristic"}},
	}

	for _, compact := range []bool{false, true} {
		var buf bytes.Buffer
		renderInfoTable(&buf, result, enr, false, compact)
		out := buf.String()
		if n := strings.Count(out, "no remote"); n != 1 {
			t.Errorf("compact=%v: %q appears %d times, want exactly 1:\n%s", compact, "no remote", n, out)
		}
	}
}

// TestRenderInfoCompact_BaseNameHoistedWhenUniform verifies the base branch is
// named once in the header instead of once per row.
func TestRenderInfoCompact_BaseNameHoistedWhenUniform(t *testing.T) {
	withColors(t, false)

	result := bulkResult(
		repository.RepositoryStatusResult{Path: "/w/a", Branch: "feat/x", Upstream: "origin/feat/x"},
		repository.RepositoryStatusResult{Path: "/w/b", Branch: "feat/y", Upstream: "origin/feat/y"},
	)
	enr := map[string]infoEnrichment{
		"/w/a": {Base: repository.BaseBranchInfo{Name: "master", Ahead: 1}},
		"/w/b": {Base: repository.BaseBranchInfo{Name: "master", Behind: 2}},
	}

	var buf bytes.Buffer
	renderInfoTable(&buf, result, enr, false, false)
	out := buf.String()

	if !strings.Contains(out, "BASE(master)") {
		t.Errorf("uniform base should be hoisted into the header:\n%s", out)
	}
	if n := strings.Count(out, "master"); n != 1 {
		t.Errorf("base name should appear exactly once (in the header), got %d:\n%s", n, out)
	}
}

// TestBaseCell_SilentOnBaseBranch covers the case that dominated the first
// draft's output: sitting on the base branch printed its name on every row to
// report a divergence that cannot be anything but zero.
func TestBaseCell_SilentOnBaseBranch(t *testing.T) {
	withColors(t, false)

	enr := infoEnrichment{Base: repository.BaseBranchInfo{Name: "master", Source: "heuristic"}}
	if got := baseCell("master", enr); got.text != "" {
		t.Errorf("base cell on the base branch = %q, want empty", got.text)
	}
	if got := baseCell("feat/x", enr); got.text != "master" {
		t.Errorf("base cell off the base branch = %q, want master", got.text)
	}
	if got := baseCell("feat/x", infoEnrichment{}); got.text != "no base" {
		t.Errorf("missing base = %q, want \"no base\"", got.text)
	}
}

// TestDivergenceCell_DistinguishesInSyncFromNoUpstream is why Upstream was
// added to RepositoryStatusResult: both states produce zero ahead/behind, but
// only one of them is fine.
func TestDivergenceCell_DistinguishesInSyncFromNoUpstream(t *testing.T) {
	withColors(t, false)

	if got := divergenceCell(0, 0, false, true); got.text != "" {
		t.Errorf("in sync = %q, want empty", got.text)
	}
	if got := divergenceCell(0, 0, true, true); got.text != "no upstream" {
		t.Errorf("untracked branch = %q, want \"no upstream\"", got.text)
	}
	if got := divergenceCell(2, 3, false, true); got.text != "↑2 ↓3" {
		t.Errorf("diverged = %q, want \"↑2 ↓3\"", got.text)
	}
}

// TestDivergenceCell_SilentWithoutARemote keeps the two columns from printing
// the same words for two different facts. A repository with no remote has
// nothing to track and nothing to push to; REMOTE says "no remote" once, and
// UPSTREAM saying it again would double the noise without adding a fact.
func TestDivergenceCell_SilentWithoutARemote(t *testing.T) {
	withColors(t, false)

	if got := divergenceCell(0, 0, true, false); got.text != "" {
		t.Errorf("no remote at all = %q, want empty (REMOTE column carries it)", got.text)
	}
}

func TestRemoteOwner(t *testing.T) {
	cases := []struct{ in, want string }{
		{"git@github.com:Gizzahub/gzh-cli.git", "github.com/Gizzahub"},
		{"https://github.com/Gizzahub/gzh-cli.git", "github.com/Gizzahub"},
		{"https://user@gitlab.com/team/proj", "gitlab.com/team"},
		{"ssh://git@gitlab.polypia.net:2224/devbox/gzh-cli-devbox.git", "gitlab.polypia.net:2224/devbox"},
		{"", ""},
		{"not-a-url", ""},
	}
	for _, tc := range cases {
		if got := remoteOwner(tc.in); got != tc.want {
			t.Errorf("remoteOwner(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestMajorityRemoteOwner_CaseInsensitive covers the workspace that actually
// motivated it: the same GitHub account spelled two ways must not read as two
// different remotes.
func TestMajorityRemoteOwner_CaseInsensitive(t *testing.T) {
	owner, count := majorityRemoteOwner([]repository.RepositoryStatusResult{
		{RemoteURL: "git@github.com:Gizzahub/a.git"},
		{RemoteURL: "git@github.com:Gizzahub/b.git"},
		{RemoteURL: "git@github.com:gizzahub/c.git"},
		{RemoteURL: "git@gitlab.com:other/d.git"},
	})
	if count != 3 {
		t.Errorf("count = %d, want 3 (case-insensitive grouping)", count)
	}
	if owner != "Gizzahub" && owner != "github.com/Gizzahub" {
		t.Errorf("owner = %q, want the dominant spelling github.com/Gizzahub", owner)
	}
	if !sameOwner("github.com/gizzahub", "github.com/Gizzahub") {
		t.Error("sameOwner must compare case-insensitively")
	}
}

func TestElideMiddle(t *testing.T) {
	if got := elideMiddle("short", 28); got != "short" {
		t.Errorf("short name should pass through, got %q", got)
	}
	long := "feature/some-extremely-long-branch-name-here"
	got := elideMiddle(long, 20)
	if len([]rune(got)) != 20 {
		t.Errorf("elided length = %d, want 20 (%q)", len([]rune(got)), got)
	}
	// Both ends carry meaning, so both must survive.
	if !strings.HasPrefix(got, "feature/") || !strings.HasSuffix(got, "here") {
		t.Errorf("elision dropped a distinguishing end: %q", got)
	}
}

// TestInfoMarker_BlockedOutranksAttention verifies the classification order: a
// repository mid-rebase cannot be acted on and must not read like one that
// merely has edits.
func TestInfoMarker_BlockedOutranksAttention(t *testing.T) {
	dirty := &repository.RepositoryStatusResult{UnstagedFiles: 3}
	if got := infoMarker(dirty, infoEnrichment{}); got != markerAttn {
		t.Errorf("dirty marker = %q, want %q", got, markerAttn)
	}

	rebasing := &repository.RepositoryStatusResult{UnstagedFiles: 3, RebaseInProgress: true}
	if got := infoMarker(rebasing, infoEnrichment{}); got != markerBlocked {
		t.Errorf("rebasing marker = %q, want %q", got, markerBlocked)
	}

	clean := &repository.RepositoryStatusResult{Upstream: "origin/main"}
	if got := infoMarker(clean, infoEnrichment{}); got != markerNone {
		t.Errorf("clean marker = %q, want blank", got)
	}
}

// TestInfoMarker_AgreesWithDirtyCell pins the invariant the first draft broke:
// the marker read TrackedChangedFiles while the DIRTY column read the
// staged/unstaged counts, so a repository could print "~3" with no marker
// beside it. Each field is exercised on its own because that is exactly the
// case a shared aggregate hides.
func TestInfoMarker_AgreesWithDirtyCell(t *testing.T) {
	withColors(t, false)

	cases := []struct {
		name string
		repo repository.RepositoryStatusResult
	}{
		{"staged only", repository.RepositoryStatusResult{StagedFiles: 1}},
		{"unstaged only", repository.RepositoryStatusResult{UnstagedFiles: 1}},
		{"untracked only", repository.RepositoryStatusResult{UntrackedFiles: 1}},
		{"stash only", repository.RepositoryStatusResult{StashCount: 1}},
		{"aggregate only", repository.RepositoryStatusResult{TrackedChangedFiles: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := tc.repo
			repo.Upstream = "origin/main"
			marker := infoMarker(&repo, infoEnrichment{})
			if marker == markerNone {
				t.Errorf("no attention marker for a dirty tree (%s)", tc.name)
			}
			// The aggregate field has no dedicated DIRTY token, so only the
			// per-category cases must also render something.
			if tc.name != "aggregate only" && dirtyCell(&repo).text == "" {
				t.Errorf("marker set but DIRTY column empty (%s)", tc.name)
			}
		})
	}
}

// TestOtherBranchesCell_WorktreeBackedFirst verifies that branches with a
// checkout behind them are named before stale ones and flagged.
func TestOtherBranchesCell_WorktreeBackedFirst(t *testing.T) {
	withColors(t, false)

	repo := &repository.RepositoryStatusResult{
		Branch:        "master",
		LocalBranches: []string{"master", "aaa-stale", "bbb-stale", "ccc-stale", "zzz-active"},
	}
	enr := infoEnrichment{
		Base:             repository.BaseBranchInfo{Name: "master"},
		WorktreeBranches: []string{"zzz-active"},
	}

	got := otherBranchesCell(repo, enr).text
	if !strings.HasPrefix(got, "zzz-active*") {
		t.Errorf("worktree-backed branch should lead and be flagged, got %q", got)
	}
	if !strings.Contains(got, "+1") {
		t.Errorf("overflow beyond %d shown branches should collapse to +N, got %q", maxOtherShown, got)
	}
	if strings.Contains(got, "master") {
		t.Errorf("current/base branch must not repeat in OTHER, got %q", got)
	}
}

func TestRemoteOnlyBranchesCell_ExcludesLocalAndSymbolicHEAD(t *testing.T) {
	withColors(t, false)

	repo := &repository.RepositoryStatusResult{
		Branch:         "master",
		Upstream:       "origin/master",
		Remotes:        map[string]string{"origin": ""},
		LocalBranches:  []string{"master", "feature/local"},
		RemoteBranches: []string{"origin/HEAD", "origin/develop", "origin/feature/local", "origin/master"},
	}
	got := remoteOnlyBranchesCell(repo, infoEnrichment{Base: repository.BaseBranchInfo{Name: "master"}}).text
	if got != "develop" {
		t.Errorf("remote-only branches = %q, want %q", got, "develop")
	}
}

func TestRemoteOnlyBranchesCell_MultipleRemotesSortAndTruncate(t *testing.T) {
	withColors(t, false)

	repo := &repository.RepositoryStatusResult{Remotes: map[string]string{"upstream": "", "origin": "", "fork": ""}, RemoteBranches: []string{
		"upstream/develop", "origin/zeta", "origin/develop", "fork/alpha", "origin/beta",
	}}
	got := remoteOnlyBranchesCell(repo, infoEnrichment{}).text
	// Full refs sort deterministically. develop occurs on two remotes, so both
	// labels retain their prefixes; the remaining entries collapse to +2.
	want := "alpha, beta, origin/develop +2"
	if got != want {
		t.Errorf("remote-only branches = %q, want %q", got, want)
	}
}

func TestRemoteOnlyBranchesCell_TopLevelBranchesSurviveTruncation(t *testing.T) {
	withColors(t, false)

	repo := &repository.RepositoryStatusResult{Remotes: map[string]string{"origin": ""}, RemoteBranches: []string{
		"origin/dependabot/github_actions/a", "origin/dependabot/github_actions/b",
		"origin/dependabot/github_actions/c", "origin/dependabot/github_actions/d",
		"origin/develop",
	}}
	got := remoteOnlyBranchesCell(repo, infoEnrichment{}).text
	if !strings.HasPrefix(got, "develop, ") || !strings.HasSuffix(got, " +2") {
		t.Errorf("top-level develop must lead a truncated remote-only list, got %q", got)
	}
}

func TestRemoteOnlyBranchesCell_UsesConfiguredSlashRemoteNames(t *testing.T) {
	withColors(t, false)

	repo := &repository.RepositoryStatusResult{
		Remotes:        map[string]string{"team/foo": "", "origin": ""},
		LocalBranches:  []string{"bar", "origin/develop"},
		RemoteBranches: []string{"team/foo/bar", "origin/develop", "origin/release"},
	}
	// bar is a local counterpart to team/foo/bar. A local branch literally
	// named origin/develop is not a counterpart to remote origin/develop.
	if got, want := remoteOnlyBranchesCell(repo, infoEnrichment{}).text, "develop, release"; got != want {
		t.Errorf("remote-only branches = %q, want %q", got, want)
	}
}

func TestRemoteOnlyBranchesCell_QualifiesAmbiguousSlashRemoteBranches(t *testing.T) {
	withColors(t, false)

	repo := &repository.RepositoryStatusResult{
		Remotes:        map[string]string{"team/foo": "", "origin": ""},
		RemoteBranches: []string{"team/foo/bar", "origin/bar"},
	}
	if got, want := remoteOnlyBranchesCell(repo, infoEnrichment{}).text, "origin/bar, team/foo/bar"; got != want {
		t.Errorf("remote-only branches = %q, want %q", got, want)
	}
}

func TestRemoteOnlyBranchesCell_ExpandsCollidingElisions(t *testing.T) {
	withColors(t, false)

	first := "very-long-remote-alpha-middle-identical-tail/develop"
	second := "very-long-remote-bravo-middle-identical-tail/develop"
	if elideMiddle(first, maxBranchWidth) != elideMiddle(second, maxBranchWidth) {
		t.Fatalf("fixture must collide after elision")
	}
	repo := &repository.RepositoryStatusResult{
		Remotes:        map[string]string{strings.TrimSuffix(first, "/develop"): "", strings.TrimSuffix(second, "/develop"): ""},
		RemoteBranches: []string{first, second},
	}
	got := remoteOnlyBranchesCell(repo, infoEnrichment{}).text
	if !strings.Contains(got, first) || !strings.Contains(got, second) {
		t.Errorf("colliding labels must retain full refs, got %q", got)
	}
}

func TestRemoteOnlyBranchesCell_ExpandsCrossKindLabelCollisions(t *testing.T) {
	withColors(t, false)

	repo := &repository.RepositoryStatusResult{
		Remotes: map[string]string{"foo": "", "up": "", "origin": ""},
		RemoteBranches: []string{
			"foo/bar", "up/bar", "origin/foo/bar", "foo/bar", // duplicate ref must not duplicate output
		},
	}
	if got, want := remoteOnlyBranchesCell(repo, infoEnrichment{}).text, "foo/bar, up/bar, origin/foo/bar"; got != want {
		t.Errorf("remote-only branches = %q, want %q", got, want)
	}
}

func TestRemoteTrackingBranch_OverlappingRemoteNamespacesUseLongestPrefix(t *testing.T) {
	remote, branch, ok := remoteTrackingBranch("team/foo/bar", map[string]string{"team": "", "team/foo": ""})
	if !ok || remote != "team/foo" || branch != "bar" {
		t.Errorf("remoteTrackingBranch() = (%q, %q, %v), want (team/foo, bar, true)", remote, branch, ok)
	}
}

func TestInfoOutputsExposeRemoteOnlyBranches(t *testing.T) {
	result := bulkResult(repository.RepositoryStatusResult{
		Path: "/w/r", RelativePath: "r", Branch: "master", Status: "clean",
		Remotes: map[string]string{"origin": ""}, LocalBranches: []string{"master"},
		RemoteBranches: []string{"origin/develop"},
	})
	enr := map[string]infoEnrichment{"/w/r": {Base: repository.BaseBranchInfo{Name: "master"}}}

	full := captureStdout(t, func() { displayInfoResultsDetailed(result, enr) })
	if !strings.Contains(full, "Remote-only (1): origin/develop") {
		t.Errorf("full output lacks remote-only field:\n%s", full)
	}

	jsonOut := captureStdout(t, func() { displayInfoResultsStructured(result, enr, "json") })
	if !strings.Contains(jsonOut, "\"remote_only_branches\": [") || !strings.Contains(jsonOut, "origin/develop") {
		t.Errorf("JSON output lacks remote_only_branches:\n%s", jsonOut)
	}

	llmOut := captureStdout(t, func() { displayInfoResultsStructured(result, enr, "llm") })
	if !strings.Contains(llmOut, "origin/develop") {
		t.Errorf("LLM output lacks remote-only branch value:\n%s", llmOut)
	}
}

func TestRenderInfoTable_CompactKeepsRemoteOnlyColumn(t *testing.T) {
	withColors(t, false)

	result := bulkResult(repository.RepositoryStatusResult{
		Path: "/w/remote-only", Branch: "master", Upstream: "origin/master",
		RemoteURL:     "git@github.com:Acme/remote-only.git",
		Remotes:       map[string]string{"origin": ""},
		LocalBranches: []string{"master"}, RemoteBranches: []string{"origin/develop"},
	})
	enr := map[string]infoEnrichment{
		"/w/remote-only": {Base: repository.BaseBranchInfo{Name: "master"}},
	}

	var buf bytes.Buffer
	renderInfoTable(&buf, result, enr, false, true)
	out := buf.String()
	if !strings.Contains(out, "REMOTE ONLY") || !strings.Contains(out, "develop") {
		t.Errorf("compact table must retain remote-only information:\n%s", out)
	}
}

func TestRenderInfoCompact_NoRepositories(t *testing.T) {
	withColors(t, false)
	var buf bytes.Buffer
	renderInfoTable(&buf, bulkResult(), nil, false, false)
	if !strings.Contains(buf.String(), "No repositories found.") {
		t.Errorf("unexpected empty-scan output: %q", buf.String())
	}
}
