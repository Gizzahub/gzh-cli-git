package integrate

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

func TestOnlyReadinessAddition(t *testing.T) {
	old := []byte("branch:\n  integrationBranch: master\n  taskPattern: dev/*\n")
	good := []byte("branch:\n  integrationBranch: master\n  taskPattern: dev/*\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n")
	if err := onlyReadinessAddition(old, good); err != nil {
		t.Fatalf("expected readiness-only addition: %v", err)
	}
	bad := []byte("branch:\n  integrationBranch: develop\n  taskPattern: dev/*\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n")
	if err := onlyReadinessAddition(old, bad); err == nil {
		t.Fatal("accepted unrelated config change")
	}
}

func TestBootstrapPlanExpiryAndShape(t *testing.T) {
	if _, err := time.Parse(time.RFC3339Nano, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapPlanApplyBareRemoteAndOneUse(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "empty-global"))
	bare := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	run := func(dir string, args ...string) string {
		t.Helper()
		c := exec.CommandContext(context.Background(), "git", args...)
		c.Dir = dir
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	os.MkdirAll(work, 0o755)
	run(root, "init", "--bare", bare)
	run(root, "clone", bare, work)
	run(work, "config", "user.email", "test@example.com")
	run(work, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(work, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: master\n"), 0o644)
	run(work, "add", ".")
	run(work, "commit", "-m", "base")
	run(work, "push", "origin", "HEAD:master")
	run(work, "checkout", "-b", "dev/bootstrap")
	os.MkdirAll(filepath.Join(work, ".gz-git", "readiness"), 0o755)
	os.WriteFile(filepath.Join(work, ".gz-git", "readiness", "check"), []byte("#!/bin/sh\nprintf '{\"version\":1,\"status\":\"ready\",\"summary\":\"ok\"}\\n'\n"), 0o755)
	os.WriteFile(filepath.Join(work, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: master\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n"), 0o644)
	run(work, "add", ".")
	run(work, "commit", "-m", "bootstrap")
	p, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/master", Issuer: "test", Expiry: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), p, work, BootstrapPlanDigest(p)); err != nil {
		t.Fatal(err)
	}
	run(work, "fetch", "origin")
	if got := run(work, "rev-parse", "origin/master"); got != p.SourceSHA {
		t.Fatalf("target=%s source=%s", got, p.SourceSHA)
	}
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), p, work, BootstrapPlanDigest(p)); err == nil {
		t.Fatal("reused bootstrap plan")
	}
}

func TestBootstrapApplyRejectsConfirmationExpiryAndFieldDrift(t *testing.T) {
	work := bootstrapFixture(t)
	p, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/master", Issuer: "test", Expiry: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), p, work, "wrong"); err == nil {
		t.Fatal("accepted wrong confirmation")
	}
	tampered := p
	tampered.PushEndpoint = "https://example.invalid/repo.git"
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), tampered, work, BootstrapPlanDigest(tampered)); err == nil {
		t.Fatal("accepted endpoint drift")
	}
	expired := p
	expired.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), expired, work, BootstrapPlanDigest(expired)); err == nil {
		t.Fatal("accepted expired plan")
	}
	longLived := p
	longLived.ExpiresAt = time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), longLived, work, BootstrapPlanDigest(longLived)); err == nil {
		t.Fatal("accepted expiry inconsistent with TTL")
	}
	future := p
	future.IssuedAt = time.Now().UTC().Add(3 * time.Minute).Format(time.RFC3339Nano)
	future.ExpiresAt = time.Now().UTC().Add(4 * time.Minute).Format(time.RFC3339Nano)
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), future, work, BootstrapPlanDigest(future)); err == nil {
		t.Fatal("accepted future-issued plan")
	}
	mutations := map[string]func(*BootstrapPlan){
		"repository":    func(v *BootstrapPlan) { v.Repository += ".other" },
		"remote":        func(v *BootstrapPlan) { v.Remote = "upstream" },
		"target-ref":    func(v *BootstrapPlan) { v.TargetRef = "origin/other" },
		"target-sha":    func(v *BootstrapPlan) { v.TargetSHA = strings.Repeat("0", 40) },
		"source-ref":    func(v *BootstrapPlan) { v.SourceRef = "other" },
		"source-sha":    func(v *BootstrapPlan) { v.SourceSHA = strings.Repeat("1", 40) },
		"manifest-path": func(v *BootstrapPlan) { v.ManifestPath += ".other" },
		"manifest-oid":  func(v *BootstrapPlan) { v.ManifestOID = strings.Repeat("2", 40) },
		"runner-path":   func(v *BootstrapPlan) { v.RunnerPath += ".other" },
		"runner-oid":    func(v *BootstrapPlan) { v.RunnerOID = strings.Repeat("3", 40) },
		"tree-oid":      func(v *BootstrapPlan) { v.ReadinessTreeOID = strings.Repeat("4", 40) },
		"tree-digest":   func(v *BootstrapPlan) { v.ReadinessTreeDigest = strings.Repeat("5", 64) },
		"destination":   func(v *BootstrapPlan) { v.DestinationRef = "refs/heads/other" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := p
			mutate(&changed)
			if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), changed, work, BootstrapPlanDigest(changed)); err == nil {
				t.Fatal("accepted changed snapshot field")
			}
		})
	}
}

func TestCanonicalPushEndpointCredentialForms(t *testing.T) {
	for _, tc := range []struct {
		in      string
		wantErr bool
	}{
		{"/tmp/remote.git", false},
		{"file:///tmp/remote.git", false},
		{"git@host:repo.git", false},
		{"ssh://git@host/repo.git", false},
		{"git://host/repo.git", false},
		{"https://user:secret@host/repo.git", true},
		{"https://TOKEN@host/repo.git", true},
		{"https://host/repo.git?access_token=secret", true},
		{"https://host/repo.git#secret", true},
		{"https://host/repo.git", false},
		{"ftp://host/repo.git", true},
		{"-bad", true},
	} {
		_, err := canonicalPushEndpoint(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("%q error=%v", tc.in, err)
		}
	}
}

func TestBootstrapRejectsUnsafeShapes(t *testing.T) {
	expect := func(name, detail string, mutate func(string, func(...string))) {
		t.Run(name, func(t *testing.T) {
			work := bootstrapFixture(t)
			run := func(args ...string) {
				c := exec.CommandContext(context.Background(), "git", args...)
				c.Dir = work
				if out, err := c.CombinedOutput(); err != nil {
					t.Fatalf("git: %v %s", err, out)
				}
			}
			mutate(work, run)
			if _, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/master", Issuer: "t", Expiry: time.Minute}); err == nil {
				t.Fatal("accepted unsafe bootstrap")
			} else if !strings.Contains(err.Error(), detail) {
				t.Fatalf("error %q does not contain %q", err, detail)
			}
		})
	}
	expect("multi-commit", "exactly one commit", func(work string, run func(...string)) {
		os.WriteFile(filepath.Join(work, "extra"), []byte("x"), 0o644)
		run("add", "extra")
		run("commit", "-m", "extra")
	})
	expect("forbidden-path", "forbidden path", func(work string, run func(...string)) {
		os.WriteFile(filepath.Join(work, "README"), []byte("x"), 0o644)
		run("add", "README")
		run("commit", "--amend", "--no-edit")
	})
	expect("config-mutation", "beyond branch.readiness", func(work string, run func(...string)) {
		os.WriteFile(filepath.Join(work, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: develop\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n"), 0o644)
		run("add", ".gz-git.yaml")
		run("commit", "--amend", "--no-edit")
	})
	expect("duplicate-yaml", "already defined", func(work string, run func(...string)) {
		os.WriteFile(filepath.Join(work, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: master\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n"), 0o644)
		run("add", ".gz-git.yaml")
		run("commit", "--amend", "--no-edit")
	})
	expect("symlink", "must be 100644 or 100755", func(work string, run func(...string)) {
		os.Remove(filepath.Join(work, ".gz-git", "readiness", "check"))
		os.Symlink("../../README", filepath.Join(work, ".gz-git", "readiness", "check"))
		run("add", "-A")
		run("commit", "--amend", "--no-edit")
	})
	expect("runner-path-mismatch", "source readiness contract missing", func(work string, run func(...string)) {
		run("mv", ".gz-git/readiness/check", ".gz-git/readiness/renamed")
		run("add", "-A")
		run("commit", "--amend", "--no-edit")
	})
	expect("manifest-mode", "mode must be unchanged", func(work string, run func(...string)) {
		if err := os.Chmod(filepath.Join(work, ".gz-git.yaml"), 0o755); err != nil {
			t.Fatal(err)
		}
		run("add", ".gz-git.yaml")
		run("commit", "--amend", "--no-edit")
	})
}

func TestBootstrapRejectsRevisionSyntaxTargets(t *testing.T) {
	work := bootstrapFixture(t)
	for _, target := range []string{"origin/refs/heads/master", "origin/master^", "origin/master@{upstream}", "refs/remotes/origin/master", "origin/HEAD"} {
		if _, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: target, Issuer: "t", Expiry: time.Minute}); err == nil {
			t.Errorf("accepted malformed target %q", target)
		}
	}
}

func TestBootstrapAcceptsSlashTargetBranch(t *testing.T) {
	work := bootstrapFixture(t)
	runGitInTest(t, work, "push", "origin", "origin/master:refs/heads/release/2.0")
	if _, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/release/2.0", Issuer: "t", Expiry: time.Minute}); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapRejectsConcurrentTargetAdvance(t *testing.T) {
	work := bootstrapFixture(t)
	p, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/master", Issuer: "t", Expiry: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other")
	runGitInTest(t, filepath.Dir(other), "clone", p.PushEndpoint, other)
	runGitInTest(t, other, "config", "user.email", "test@example.com")
	runGitInTest(t, other, "config", "user.name", "Test")
	runGitInTest(t, other, "checkout", "-b", "master", "origin/master")
	if err := os.WriteFile(filepath.Join(other, "concurrent"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitInTest(t, other, "add", "concurrent")
	runGitInTest(t, other, "commit", "-m", "concurrent")
	runGitInTest(t, other, "push", "origin", "master")
	want := runGitInTest(t, other, "rev-parse", "HEAD")
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), p, work, BootstrapPlanDigest(p)); err == nil {
		t.Fatal("accepted stale target")
	}
	if got := strings.Fields(runGitInTest(t, work, "ls-remote", p.PushEndpoint, p.DestinationRef))[0]; got != want {
		t.Fatalf("remote moved: got %s want %s", got, want)
	}
}

func runGitInTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.CommandContext(context.Background(), "git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestBootstrapRejectsAmbiguousPushURL(t *testing.T) {
	work := bootstrapFixture(t)
	for _, value := range []string{"/tmp/one.git", "/tmp/two.git"} {
		c := exec.CommandContext(context.Background(), "git", "config", "--add", "remote.origin.pushurl", value)
		c.Dir = work
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	if _, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/master", Issuer: "t", Expiry: time.Minute}); err == nil {
		t.Fatal("accepted ambiguous pushurl")
	}
}

func TestBootstrapAllowsUnrelatedURLRewrite(t *testing.T) {
	work := bootstrapFixture(t)
	runGitInTest(t, work, "config", "url.git@github.com:.insteadOf", "https://github.com/")
	if _, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/master", Issuer: "t", Expiry: time.Minute}); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapUsesPushURLTargetAndDestination(t *testing.T) {
	work := bootstrapFixture(t)
	fetchEndpoint := runGitInTest(t, work, "remote", "get-url", "origin")
	pushEndpoint := filepath.Join(t.TempDir(), "push.git")
	runGitInTest(t, filepath.Dir(pushEndpoint), "clone", "--mirror", fetchEndpoint, pushEndpoint)
	runGitInTest(t, work, "config", "remote.origin.pushurl", pushEndpoint)
	p, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/master", Issuer: "t", Expiry: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if p.PushEndpoint != pushEndpoint {
		t.Fatalf("push endpoint=%q want %q", p.PushEndpoint, pushEndpoint)
	}
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), p, work, BootstrapPlanDigest(p)); err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(runGitInTest(t, work, "ls-remote", pushEndpoint, p.DestinationRef))[0]; got != p.SourceSHA {
		t.Fatalf("push target=%s source=%s", got, p.SourceSHA)
	}
	if got := strings.Fields(runGitInTest(t, work, "ls-remote", fetchEndpoint, p.DestinationRef))[0]; got != p.TargetSHA {
		t.Fatalf("fetch endpoint unexpectedly changed: %s", got)
	}
}

func TestBootstrapLeaseRejectsPushTimeRace(t *testing.T) {
	work := bootstrapFixture(t)
	p, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/master", Issuer: "t", Expiry: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other")
	runGitInTest(t, filepath.Dir(other), "clone", p.PushEndpoint, other)
	runGitInTest(t, other, "config", "user.email", "test@example.com")
	runGitInTest(t, other, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(other, "concurrent"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitInTest(t, other, "add", "concurrent")
	runGitInTest(t, other, "commit", "-m", "concurrent")
	concurrent := runGitInTest(t, other, "rev-parse", "HEAD")
	runGitInTest(t, other, "push", "origin", "HEAD:refs/heads/concurrent-bootstrap-test")
	hook := filepath.Join(work, ".git", "hooks", "pre-push")
	script := "#!/bin/sh\ngit --git-dir='" + p.PushEndpoint + "' update-ref '" + p.DestinationRef + "' '" + concurrent + "'\n"
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapApply(context.Background(), gitcmd.NewExecutor(), p, work, BootstrapPlanDigest(p)); err == nil {
		t.Fatal("lease accepted push-time race")
	}
	if got := strings.Fields(runGitInTest(t, work, "ls-remote", p.PushEndpoint, p.DestinationRef))[0]; got != concurrent {
		t.Fatalf("concurrent target overwritten: got %s want %s", got, concurrent)
	}
}

func TestReadBootstrapPlanIsStrict(t *testing.T) {
	work := bootstrapFixture(t)
	p, err := BootstrapPlanFor(context.Background(), gitcmd.NewExecutor(), BootstrapOptions{RepoPath: work, Target: "origin/master", Issuer: "t", Expiry: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	valid, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(valid, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "runner_oid")
	missing, _ := json.Marshal(fields)
	cases := map[string][]byte{
		"unknown":   append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"unknown":true}`)...),
		"duplicate": append([]byte(`{"version":1,`), valid[1:]...),
		"missing":   missing,
		"trailing":  append(append([]byte{}, valid...), []byte(` {}`)...),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plan.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadBootstrapPlan(path); err == nil {
				t.Fatal("accepted malformed plan JSON")
			}
		})
	}
}

func bootstrapFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "empty-global"))
	bare := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	os.MkdirAll(work, 0o755)
	run := func(dir string, args ...string) {
		c := exec.CommandContext(context.Background(), "git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run(root, "init", "--bare", bare)
	run(root, "clone", bare, work)
	run(work, "config", "user.email", "test@example.com")
	run(work, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(work, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: master\n"), 0o644)
	run(work, "add", ".")
	run(work, "commit", "-m", "base")
	run(work, "push", "origin", "HEAD:master")
	run(work, "checkout", "-b", "dev/bootstrap")
	os.MkdirAll(filepath.Join(work, ".gz-git", "readiness"), 0o755)
	os.WriteFile(filepath.Join(work, ".gz-git", "readiness", "check"), []byte("#!/bin/sh\nprintf '{\"version\":1,\"status\":\"ready\",\"summary\":\"ok\"}\\n'\n"), 0o755)
	os.WriteFile(filepath.Join(work, ".gz-git.yaml"), []byte("branch:\n  integrationBranch: master\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/check\n"), 0o644)
	run(work, "add", ".")
	run(work, "commit", "-m", "bootstrap")
	return work
}
