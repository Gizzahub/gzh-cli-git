package cmd

import (
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/workspacecli"
)

func TestUpdateHasSkipFetchAndDeprecatedNoFetch(t *testing.T) {
	if updateCmd.Flags().Lookup("skip-fetch") == nil {
		t.Fatal("update missing --skip-fetch")
	}
	noFetch := updateCmd.Flags().Lookup("no-fetch")
	if noFetch == nil {
		t.Fatal("update missing deprecated --no-fetch alias")
	}
	if noFetch.Deprecated == "" {
		t.Error("expected --no-fetch to be marked deprecated")
	}
}

func TestWorkspaceSyncRecurseWorkspacesFlag(t *testing.T) {
	f := workspacecli.CommandFactory{}
	root := f.NewRootCmd()
	syncCmd, _, err := root.Find([]string{"sync"})
	if err != nil {
		t.Fatalf("find sync: %v", err)
	}
	if syncCmd.Flags().Lookup("recurse-workspaces") == nil {
		t.Fatal("workspace sync missing --recurse-workspaces")
	}
	rec := syncCmd.Flags().Lookup("recursive")
	if rec == nil {
		t.Fatal("workspace sync missing deprecated --recursive alias")
	}
	if rec.Deprecated == "" {
		t.Error("expected --recursive to be marked deprecated")
	}
}

func TestCommandGroupsCoreIncludesBranchStashTagWorktree(t *testing.T) {
	setCommandGroups(rootCmd)
	wantCore := map[string]bool{
		"branch": true, "stash": true, "tag": true, "worktree": true,
	}
	for _, c := range rootCmd.Commands() {
		if !wantCore[c.Name()] {
			continue
		}
		if c.GroupID != "core" {
			t.Errorf("%s GroupID=%q want core", c.Name(), c.GroupID)
		}
		delete(wantCore, c.Name())
	}
	for name := range wantCore {
		t.Errorf("command %s not found under root", name)
	}
}

func TestCleanupBranchHasBotsAndFormatFlags(t *testing.T) {
	if cleanupBranchCmd.Flags().Lookup("bots") == nil {
		t.Fatal("cleanup branch missing --bots")
	}
	if cleanupBranchCmd.Flags().Lookup("format") == nil {
		t.Fatal("cleanup branch missing --format")
	}
	long := cleanupBranchCmd.Long
	if !strings.Contains(long, "--bots --merged -r") {
		t.Errorf("cleanup branch help missing bot remote example:\n%s", long)
	}
}

func TestBranchHelpMentionsCreateDeletePolicy(t *testing.T) {
	long := branchCmd.Long
	if !strings.Contains(long, "create/delete") && !strings.Contains(long, "create") {
		t.Errorf("branch help missing create/delete policy:\n%s", long)
	}
	if !strings.Contains(long, "cleanup branch") {
		t.Errorf("branch help should mention cleanup branch:\n%s", long)
	}
}

func TestRootHelpMentionsTwoEngines(t *testing.T) {
	long := rootCmd.Long
	if !strings.Contains(long, "Ad-hoc scan") || !strings.Contains(long, "Declarative") {
		t.Errorf("root help missing two-engine distinction:\n%s", long)
	}
}
