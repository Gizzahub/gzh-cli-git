package contextref

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil/builders"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

const sampleManifest = "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: README.md\n"

func TestObserveFourStateMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		setup   func(*testing.T, *builders.GitRepoBuilder) string
		reason  string
		tracked bool
		present bool
	}{
		{
			name: "untracked absent",
			setup: func(t *testing.T, b *builders.GitRepoBuilder) string {
				t.Helper()
				return b.WithFile("README.md", "hi\n").Build()
			},
			reason: ReasonNotDeclared,
		},
		{
			name: "untracked present",
			setup: func(t *testing.T, b *builders.GitRepoBuilder) string {
				t.Helper()
				dir := b.WithFile("README.md", "hi\n").Build()
				writeFile(t, filepath.Join(dir, ManifestFile), sampleManifest)
				return dir
			},
			reason:  ReasonUntracked,
			present: true,
		},
		{
			name: "tracked absent",
			setup: func(t *testing.T, b *builders.GitRepoBuilder) string {
				t.Helper()
				dir := b.WithFile("README.md", "hi\n").WithFile(ManifestFile, sampleManifest).Build()
				if err := os.Remove(filepath.Join(dir, ManifestFile)); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			reason:  ReasonMissingFile,
			tracked: true,
		},
		{
			name: "tracked present",
			setup: func(t *testing.T, b *builders.GitRepoBuilder) string {
				t.Helper()
				return b.WithFile("README.md", "hi\n").WithFile(ManifestFile, sampleManifest).Build()
			},
			tracked: true,
			present: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := tt.setup(t, builders.NewGitRepoBuilder(t))
			obs, err := Observe(context.Background(), Options{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if obs.Context.ComponentOutcome != OutcomeObserved {
				t.Fatalf("outcome = %s, want observed", obs.Context.ComponentOutcome)
			}
			if obs.Context.Reason != tt.reason {
				t.Fatalf("reason = %q, want %q", obs.Context.Reason, tt.reason)
			}
			if strings.Contains(obs.Context.Reason, "native-discovery") {
				t.Fatal("absence must not be native-discovery")
			}
			if obs.Context.Manifest == nil {
				t.Fatal("manifest state missing")
			}
			if obs.Context.Manifest.Tracked != tt.tracked || obs.Context.Manifest.Present != tt.present {
				t.Fatalf("tracked/present = %v/%v, want %v/%v",
					obs.Context.Manifest.Tracked, obs.Context.Manifest.Present, tt.tracked, tt.present)
			}
			if obs.ExitCode != cliutil.ExitOK {
				t.Fatalf("exit = %d, want 0", obs.ExitCode)
			}
			if obs.ReleasedCETag != ReleasedCETag || obs.ReleasedCECommit != ReleasedCECommit {
				t.Fatalf("release citation = %s %s", obs.ReleasedCETag, obs.ReleasedCECommit)
			}
			if obs.CE.ComponentOutcome != OutcomeAbsent {
				t.Fatalf("ce outcome = %s, want absent", obs.CE.ComponentOutcome)
			}
		})
	}
}

func TestObserveIgnoresNonRootManifest(t *testing.T) {
	t.Parallel()
	dir := builders.NewGitRepoBuilder(t).WithFile("README.md", "hi\n").Build()
	nested := filepath.Join(dir, "nested", ManifestFile)
	if err := os.MkdirAll(filepath.Dir(nested), 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, nested, sampleManifest)
	obs, err := Observe(context.Background(), Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if obs.Context.Reason != ReasonNotDeclared {
		t.Fatalf("reason = %q, want %s", obs.Context.Reason, ReasonNotDeclared)
	}
}

func TestObserveDoesNotMutateRepo(t *testing.T) {
	t.Parallel()
	dir := builders.NewGitRepoBuilder(t).
		WithFile("README.md", "hi\n").
		WithFile(ManifestFile, sampleManifest).
		Build()
	before := snapshotRepo(t, dir)
	if _, err := Observe(context.Background(), Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	after := snapshotRepo(t, dir)
	if before != after {
		t.Fatalf("repository mutated\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestObserveEntrypointDirtyAndIndexOnly(t *testing.T) {
	t.Parallel()
	dir := builders.NewGitRepoBuilder(t).
		WithFile("README.md", "hi\n").
		WithFile("keep.md", "keep\n").
		WithFile(ManifestFile, "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: README.md\n    - path: keep.md\n").
		Build()
	writeFile(t, filepath.Join(dir, "README.md"), "dirty\n")
	if err := os.Remove(filepath.Join(dir, "keep.md")); err != nil {
		t.Fatal(err)
	}
	obs, err := Observe(context.Background(), Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Context.Entrypoints) != 2 {
		t.Fatalf("entrypoints = %d", len(obs.Context.Entrypoints))
	}
	byPath := map[string]Entrypoint{}
	for _, ep := range obs.Context.Entrypoints {
		byPath[ep.Path] = ep
	}
	if !byPath["README.md"].Dirty || byPath["README.md"].Reason != ReasonDirty {
		t.Fatalf("README.md = %+v", byPath["README.md"])
	}
	if !byPath["keep.md"].IndexOnly || byPath["keep.md"].Reason != ReasonIndexOnly {
		t.Fatalf("keep.md = %+v", byPath["keep.md"])
	}
}

func TestObserveLiteralPathspecNotGlob(t *testing.T) {
	t.Parallel()
	dir := builders.NewGitRepoBuilder(t).
		WithFile("a.md", "plain\n").
		WithFile("[a].md", "bracket\n").
		WithFile(ManifestFile, "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: \"[a].md\"\n").
		Build()
	obs, err := Observe(context.Background(), Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Context.Entrypoints) != 1 || obs.Context.Entrypoints[0].Path != "[a].md" {
		t.Fatalf("entrypoints = %+v", obs.Context.Entrypoints)
	}
	ep := obs.Context.Entrypoints[0]
	if ep.Reason != "" || ep.WorktreeDigest == "" {
		t.Fatalf("entrypoint = %+v", ep)
	}
	want := contentDigest([]byte("bracket\n"))
	if ep.WorktreeDigest != want {
		t.Fatalf("digest = %s, want %s (must not hash a.md)", ep.WorktreeDigest, want)
	}
}

func TestObserveRejectsWorktreeSymlinkWithoutFollow(t *testing.T) {
	t.Parallel()
	dir := builders.NewGitRepoBuilder(t).
		WithFile("README.md", "secret-target\n").
		WithFile("keep.md", "keep\n").
		WithFile(ManifestFile, "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: README.md\n    - path: docs/SOUL.md\n").
		Build()
	if err := os.Remove(filepath.Join(dir, "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("keep.md", filepath.Join(dir, "README.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	docs := filepath.Join(dir, "docs")
	if err := os.Mkdir(docs, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(docs, "SOUL.md"), "soul\n")
	runGit(t, dir, "add", "docs/SOUL.md")
	runGit(t, dir, "commit", "-m", "add soul")
	if err := os.RemoveAll(docs); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", docs); err != nil {
		t.Fatal(err)
	}
	obs, err := Observe(context.Background(), Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]Entrypoint{}
	for _, ep := range obs.Context.Entrypoints {
		byPath[ep.Path] = ep
	}
	if byPath["README.md"].Reason != ReasonSymlink || byPath["README.md"].WorktreeDigest != "" {
		t.Fatalf("README.md = %+v", byPath["README.md"])
	}
	if byPath["docs/SOUL.md"].Reason != ReasonSymlink || byPath["docs/SOUL.md"].WorktreeDigest != "" {
		t.Fatalf("docs/SOUL.md = %+v", byPath["docs/SOUL.md"])
	}
}

func TestObserveCitesReleasedCEContract(t *testing.T) {
	t.Parallel()
	if ReleasedCETag != "v0.8.3" {
		t.Fatalf("ReleasedCETag = %s, want v0.8.3", ReleasedCETag)
	}
	if ReleasedCECommit != "ac7445978423df45cb77ffaea0e34f7725e744b2" {
		t.Fatalf("ReleasedCECommit = %s", ReleasedCECommit)
	}
	if strings.Contains(ReleasedCECommit, "de0d8a8b") || strings.Contains(ReleasedCECommit, "c4913da6") {
		t.Fatal("must not cite de0d8a8b or TASK-083")
	}
}

func TestObserveCEPassFindingFaultAndDisagree(t *testing.T) {
	t.Parallel()
	dir := builders.NewGitRepoBuilder(t).WithFile("README.md", "hi\n").Build()
	tests := []struct {
		scenario string
		outcome  string
		domain   string
		status   string
		exit     int
	}{
		{scenario: "pass", outcome: OutcomeObserved, status: "adopted", exit: 0},
		{scenario: "finding", outcome: OutcomeObserved, status: "not-adopted", exit: 0},
		{scenario: "fault", outcome: OutcomeFault, domain: DomainCEInvocation, exit: 1},
		{scenario: "disagree-0", outcome: OutcomeFault, domain: DomainTransport, exit: 1},
		{scenario: "disagree-1", outcome: OutcomeFault, domain: DomainTransport, exit: 1},
		{scenario: "extra-json", outcome: OutcomeFault, domain: DomainTransport, exit: 1},
		{scenario: "overflow", outcome: OutcomeFault, domain: DomainTransport, exit: 1},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			bin, digest := fakeCEWithScenario(t, tt.scenario)
			obs, err := Observe(context.Background(), Options{
				Dir: dir,
				CE:  &CEDescriptor{Path: bin, Digest: digest},
			})
			if err != nil {
				t.Fatal(err)
			}
			if obs.CE.ComponentOutcome != tt.outcome {
				t.Fatalf("outcome = %s, want %s (%s)", obs.CE.ComponentOutcome, tt.outcome, obs.CE.Reason)
			}
			if tt.domain != "" && obs.CE.FaultDomain != tt.domain {
				t.Fatalf("domain = %s, want %s", obs.CE.FaultDomain, tt.domain)
			}
			if tt.status != "" && obs.CE.ProviderStatus != tt.status {
				t.Fatalf("status = %s, want %s", obs.CE.ProviderStatus, tt.status)
			}
			if obs.ExitCode != tt.exit {
				t.Fatalf("exit = %d, want %d", obs.ExitCode, tt.exit)
			}
			if obs.ExitCode == cliutil.ExitPartialFailed || obs.ExitCode == cliutil.ExitReclaimIncomplete {
				t.Fatal("forbidden gz-git exit 2/3")
			}
		})
	}
}

func TestObserveCETimeout(t *testing.T) {
	t.Parallel()
	dir := builders.NewGitRepoBuilder(t).WithFile("README.md", "hi\n").Build()
	bin, digest := fakeCEWithScenario(t, "hang")
	obs, err := Observe(context.Background(), Options{
		Dir:     dir,
		Timeout: 500 * time.Millisecond,
		CE:      &CEDescriptor{Path: bin, Digest: digest},
	})
	if err != nil {
		t.Fatal(err)
	}
	if obs.CE.ComponentOutcome != OutcomeFault || obs.ExitCode != cliutil.ExitToolError {
		t.Fatalf("timeout outcome=%s exit=%d reason=%s", obs.CE.ComponentOutcome, obs.ExitCode, obs.CE.Reason)
	}
}

var (
	fakeCEOnce   sync.Once
	fakeCEBin    string
	fakeCEDigest string
	errFakeCE    error
)

func fakeCE(t *testing.T) (bin, digest string) {
	t.Helper()
	fakeCEOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gzgit-fakece-")
		if err != nil {
			errFakeCE = err
			return
		}
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			errFakeCE = errNotGit
			return
		}
		src := filepath.Join(filepath.Dir(thisFile), "testdata", "fakece")
		built := filepath.Join(dir, "ce")
		cmd := exec.CommandContext(context.Background(), "go", "build", "-o", built, ".")
		cmd.Dir = src
		out, err := cmd.CombinedOutput()
		if err != nil {
			errFakeCE = fmt.Errorf("go build fakece: %w\n%s", err, out)
			return
		}
		sum, err := os.ReadFile(built)
		if err != nil {
			errFakeCE = err
			return
		}
		d := sha256.Sum256(sum)
		fakeCEBin = built
		fakeCEDigest = "sha256:" + hex.EncodeToString(d[:])
	})
	if errFakeCE != nil {
		t.Fatal(errFakeCE)
	}
	return fakeCEBin, fakeCEDigest
}

func fakeCEWithScenario(t *testing.T, scenario string) (bin, digest string) {
	t.Helper()
	src, digest := fakeCE(t)
	dir := t.TempDir()
	bin = filepath.Join(dir, "ce")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, data, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "scenario"), scenario)
	return bin, digest
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func snapshotRepo(t *testing.T, dir string) string {
	t.Helper()
	cmds := [][]string{
		{"status", "--porcelain=v1"},
		{"diff-index", "HEAD"},
		{"ls-files", "-s"},
		{"config", "--local", "--list"},
	}
	var b strings.Builder
	for _, args := range cmds {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		b.WriteString(strings.Join(args, " "))
		b.WriteByte('\n')
		b.Write(out)
	}
	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if strings.HasPrefix(rel, ".git"+string(filepath.Separator)) || rel == ".git" {
			return nil
		}
		b.WriteString(rel)
		b.WriteByte(' ')
		b.WriteString(info.Mode().String())
		b.WriteByte('\n')
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	return b.String()
}
