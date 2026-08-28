package integrate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

func TestValidateUniqueManifestYAMLDeepTraversal(t *testing.T) {
	const depth = 128
	var unique strings.Builder
	for i := 0; i < depth; i++ {
		unique.WriteString(strings.Repeat("  ", i))
		unique.WriteString("next:\n")
	}
	unique.WriteString(strings.Repeat("  ", depth))
	unique.WriteString("leaf: ok\n")
	if err := validateUniqueManifest([]byte(unique.String()), false); err != nil {
		t.Fatalf("rejected unique deep YAML: %v", err)
	}

	var duplicate strings.Builder
	for i := 0; i < depth; i++ {
		duplicate.WriteString(strings.Repeat("  ", i))
		duplicate.WriteString("next:\n")
	}
	duplicate.WriteString(strings.Repeat("  ", depth))
	duplicate.WriteString("leaf: one\n")
	duplicate.WriteString(strings.Repeat("  ", depth))
	duplicate.WriteString("leaf: two\n")
	if err := validateUniqueManifest([]byte(duplicate.String()), false); err == nil {
		t.Fatal("accepted deep duplicate YAML key")
	}
}

func TestReadinessUpdatePlanApplyRenamedRunnerAndDeletedHelper(t *testing.T) {
	work := readinessUpdateFixture(t, ".gz-git.yaml")
	runGitInTest(t, work, "mv", ".gz-git/readiness/check", ".gz-git/readiness/check-v2")
	if err := os.Remove(filepath.Join(work, ".gz-git", "readiness", "helper")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: master\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check-v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitInTest(t, work, "add", "-A")
	runGitInTest(t, work, "commit", "-m", "update readiness")
	p, err := ReadinessUpdatePlanFor(context.Background(), gitcmd.NewExecutor(), ReadinessUpdateOptions{RepoPath: work, Target: "origin/master", Issuer: "human", Expiry: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if p.OldRunnerPath != ".gz-git/readiness/check" || p.NewRunnerPath != ".gz-git/readiness/check-v2" {
		t.Fatalf("runner paths: %#v", p)
	}
	if err := ReadinessUpdateApply(context.Background(), gitcmd.NewExecutor(), p, work, ReadinessUpdatePlanDigest(p)); err != nil {
		t.Fatal(err)
	}
	if got := runGitInTest(t, work, "ls-remote", p.PushEndpoint, p.DestinationRef); !strings.HasPrefix(got, p.SourceSHA) {
		t.Fatalf("target=%q source=%s", got, p.SourceSHA)
	}
	if err := ReadinessUpdateApply(context.Background(), gitcmd.NewExecutor(), p, work, ReadinessUpdatePlanDigest(p)); err == nil {
		t.Fatal("reused readiness update plan")
	}
}

func TestReadinessUpdateSupportsYAMLYMLAndJSON(t *testing.T) {
	for _, manifest := range []string{".gz-git.yaml", ".gz-git.yml", ".gz-git.json"} {
		t.Run(manifest, func(t *testing.T) {
			work := readinessUpdateFixture(t, manifest)
			writeUpdateManifest(t, work, manifest, ".gz-git/readiness/v2")
			runGitInTest(t, work, "mv", ".gz-git/readiness/check", ".gz-git/readiness/v2")
			runGitInTest(t, work, "add", "-A")
			runGitInTest(t, work, "commit", "-m", "rename readiness")
			if _, err := ReadinessUpdatePlanFor(context.Background(), gitcmd.NewExecutor(), ReadinessUpdateOptions{RepoPath: work, Target: "origin/master", Issuer: "human", Expiry: time.Minute}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReadinessUpdateRejectsUnsafeDiffAndDuplicateManifest(t *testing.T) {
	for name, mutate := range map[string]func(t *testing.T, work string){
		"forbidden path": func(t *testing.T, work string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(work, "README"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGitInTest(t, work, "add", "README")
		},
		"symlink": func(t *testing.T, work string) {
			t.Helper()
			if err := os.Remove(filepath.Join(work, ".gz-git", "readiness", "check")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../../README", filepath.Join(work, ".gz-git", "readiness", "check")); err != nil {
				t.Fatal(err)
			}
			runGitInTest(t, work, "add", "-A")
		},
		"duplicate manifest": func(t *testing.T, work string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(work, ".gz-git.yml"), []byte("branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGitInTest(t, work, "add", ".gz-git.yml")
		},
	} {
		t.Run(name, func(t *testing.T) {
			work := readinessUpdateFixture(t, ".gz-git.yaml")
			mutate(t, work)
			runGitInTest(t, work, "commit", "-m", "unsafe update")
			if _, err := ReadinessUpdatePlanFor(context.Background(), gitcmd.NewExecutor(), ReadinessUpdateOptions{RepoPath: work, Target: "origin/master", Issuer: "human", Expiry: time.Minute}); err == nil {
				t.Fatal("accepted unsafe update")
			}
		})
	}
}

func TestReadReadinessUpdatePlanIsStrict(t *testing.T) {
	work := readinessUpdateFixture(t, ".gz-git.yaml")
	writeUpdateManifest(t, work, ".gz-git.yaml", ".gz-git/readiness/v2")
	runGitInTest(t, work, "mv", ".gz-git/readiness/check", ".gz-git/readiness/v2")
	runGitInTest(t, work, "add", "-A")
	runGitInTest(t, work, "commit", "-m", "update")
	p, err := ReadinessUpdatePlanFor(context.Background(), gitcmd.NewExecutor(), ReadinessUpdateOptions{RepoPath: work, Target: "origin/master", Issuer: "human", Expiry: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"unknown": append(append([]byte{}, b[:len(b)-1]...), []byte(`,"x":true}`)...), "duplicate": append([]byte(`{"version":1,`), b[1:]...)} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plan.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadReadinessUpdatePlan(path); err == nil {
				t.Fatal("accepted non-strict plan")
			}
		})
	}
}

func TestReadinessUpdateApplyRejectsTamperExpiryDriftAndReplay(t *testing.T) {
	work := readinessUpdateFixture(t, ".gz-git.yaml")
	writeUpdateManifest(t, work, ".gz-git.yaml", ".gz-git/readiness/v2")
	runGitInTest(t, work, "mv", ".gz-git/readiness/check", ".gz-git/readiness/v2")
	runGitInTest(t, work, "add", "-A")
	runGitInTest(t, work, "commit", "-m", "update")
	p, err := ReadinessUpdatePlanFor(context.Background(), gitcmd.NewExecutor(), ReadinessUpdateOptions{RepoPath: work, Target: "origin/master", Issuer: "human", Expiry: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if p.OldContractDigest == "" || p.NewContractDigest == "" || p.OldReadinessTreePath != ".gz-git/readiness" || p.NewReadinessTreePath != ".gz-git/readiness" {
		t.Fatalf("missing contract evidence: %#v", p)
	}
	if err := ReadinessUpdateApply(context.Background(), gitcmd.NewExecutor(), p, work, "wrong"); err == nil {
		t.Fatal("accepted wrong confirmation")
	}
	for name, mutate := range map[string]func(*ReadinessUpdatePlan){
		"endpoint":  func(v *ReadinessUpdatePlan) { v.PushEndpoint = "https://example.invalid/repo.git" },
		"tree path": func(v *ReadinessUpdatePlan) { v.NewReadinessTreePath = "other" },
		"digest":    func(v *ReadinessUpdatePlan) { v.NewContractDigest = strings.Repeat("0", 64) },
		"operation": func(v *ReadinessUpdatePlan) { v.OperationRef = "refs/heads/x" },
	} {
		t.Run(name, func(t *testing.T) {
			v := p
			mutate(&v)
			if err := ReadinessUpdateApply(context.Background(), gitcmd.NewExecutor(), v, work, ReadinessUpdatePlanDigest(v)); err == nil {
				t.Fatal("accepted tampered plan")
			}
		})
	}
	expired := p
	expired.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := ReadinessUpdateApply(context.Background(), gitcmd.NewExecutor(), expired, work, ReadinessUpdatePlanDigest(expired)); err == nil {
		t.Fatal("accepted expired plan")
	}
	future := p
	future.IssuedAt = time.Now().UTC().Add(3 * time.Minute).Format(time.RFC3339Nano)
	future.ExpiresAt = time.Now().UTC().Add(4 * time.Minute).Format(time.RFC3339Nano)
	if err := ReadinessUpdateApply(context.Background(), gitcmd.NewExecutor(), future, work, ReadinessUpdatePlanDigest(future)); err == nil {
		t.Fatal("accepted future plan")
	}
	if err := ReadinessUpdateApply(context.Background(), gitcmd.NewExecutor(), p, work, ReadinessUpdatePlanDigest(p)); err != nil {
		t.Fatal(err)
	}
	// Rolling the target back cannot replay the plan: the independent operation
	// marker was atomically persisted at the remote.
	runGitInTest(t, filepath.Dir(p.PushEndpoint), "--git-dir="+p.PushEndpoint, "update-ref", p.DestinationRef, p.TargetSHA)
	if err := ReadinessUpdateApply(context.Background(), gitcmd.NewExecutor(), p, work, ReadinessUpdatePlanDigest(p)); err == nil {
		t.Fatal("accepted replay after target rollback")
	}
}

func TestReadinessUpdateRejectsSourceAndTargetRaces(t *testing.T) {
	for name, race := range map[string]func(t *testing.T, work string, p ReadinessUpdatePlan){
		"source": func(t *testing.T, work string, _ ReadinessUpdatePlan) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(work, ".gz-git", "readiness", "extra"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGitInTest(t, work, "add", ".gz-git/readiness/extra")
			runGitInTest(t, work, "commit", "-m", "source race")
		},
		"target": func(t *testing.T, _ string, p ReadinessUpdatePlan) {
			t.Helper()
			other := filepath.Join(t.TempDir(), "other")
			runGitInTest(t, filepath.Dir(other), "clone", p.PushEndpoint, other)
			runGitInTest(t, other, "config", "user.email", "test@example.com")
			runGitInTest(t, other, "config", "user.name", "Test")
			runGitInTest(t, other, "checkout", "-b", "master", "origin/master")
			if err := os.WriteFile(filepath.Join(other, "race"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGitInTest(t, other, "add", "race")
			runGitInTest(t, other, "commit", "-m", "target race")
			runGitInTest(t, other, "push", "origin", "HEAD:master")
		},
	} {
		t.Run(name, func(t *testing.T) {
			work := readinessUpdateFixture(t, ".gz-git.yaml")
			writeUpdateManifest(t, work, ".gz-git.yaml", ".gz-git/readiness/v2")
			runGitInTest(t, work, "mv", ".gz-git/readiness/check", ".gz-git/readiness/v2")
			runGitInTest(t, work, "add", "-A")
			runGitInTest(t, work, "commit", "-m", "update")
			p, err := ReadinessUpdatePlanFor(context.Background(), gitcmd.NewExecutor(), ReadinessUpdateOptions{RepoPath: work, Target: "origin/master", Issuer: "human", Expiry: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			race(t, work, p)
			if err := ReadinessUpdateApply(context.Background(), gitcmd.NewExecutor(), p, work, ReadinessUpdatePlanDigest(p)); err == nil {
				t.Fatal("accepted raced state")
			}
		})
	}
}

func readinessUpdateFixture(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "empty-global"))
	bare, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitInTest(t, root, "init", "--bare", bare)
	runGitInTest(t, root, "clone", bare, work)
	runGitInTest(t, work, "config", "user.email", "test@example.com")
	runGitInTest(t, work, "config", "user.name", "Test")
	if err := os.MkdirAll(filepath.Join(work, ".gz-git", "readiness"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeUpdateManifest(t, work, manifest, ".gz-git/readiness/check")
	if err := os.WriteFile(filepath.Join(work, ".gz-git", "readiness", "check"), []byte("#!/bin/sh\nprintf '{\"version\":1,\"status\":\"ready\",\"summary\":\"ok\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".gz-git", "readiness", "helper"), []byte("helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitInTest(t, work, "add", ".")
	runGitInTest(t, work, "commit", "-m", "target readiness")
	runGitInTest(t, work, "push", "origin", "HEAD:master")
	runGitInTest(t, work, "checkout", "-b", "dev/update")
	return work
}

func writeUpdateManifest(t *testing.T, work, manifest, runner string) {
	t.Helper()
	var data string
	if strings.HasSuffix(manifest, ".json") {
		data = `{"branch":{"integrationBranch":"master","readiness":{"version":1,"runner":"` + runner + `"}}}`
	} else {
		data = "branch:\n  integrationBranch: master\n  readiness:\n    version: 1\n    runner: " + runner + "\n"
	}
	if err := os.WriteFile(filepath.Join(work, manifest), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
