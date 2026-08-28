package integrate

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"gopkg.in/yaml.v3"
)

// BootstrapPlan is an immutable, auditable confirmation plan for the
// one-commit readiness bootstrap. It is not a signed authorization; apply
// requires an explicit human confirmation and recomputes every field.
type BootstrapPlan struct {
	Version             int    `json:"version"`
	OperationID         string `json:"operation_id"`
	Issuer              string `json:"issuer"`
	ExpiresAt           string `json:"expires_at"`
	Repository          string `json:"repository"`
	Remote              string `json:"remote"`
	TargetRef           string `json:"target_ref"`
	TargetSHA           string `json:"target_sha"`
	SourceRef           string `json:"source_ref"`
	SourceSHA           string `json:"source_sha"`
	ManifestPath        string `json:"manifest_path"`
	ManifestOID         string `json:"manifest_oid"`
	RunnerPath          string `json:"runner_path"`
	RunnerOID           string `json:"runner_oid"`
	ReadinessTreeOID    string `json:"readiness_tree_oid"`
	ReadinessTreeDigest string `json:"readiness_tree_digest"`
	PushEndpoint        string `json:"push_endpoint"`
	DestinationRef      string `json:"destination_ref"`
	IssuedAt            string `json:"issued_at"`
	TTLSeconds          int64  `json:"ttl_seconds"`
}

// BootstrapOptions selects the source and exact target for a confirmation plan.
type BootstrapOptions struct {
	RepoPath, Branch, Target, Issuer string
	Expiry                           time.Duration
	PlanPath                         string
}

// BootstrapPlanFor validates a one-commit readiness introduction and returns
// the exact expiring facts that a human must review before apply.
func BootstrapPlanFor(ctx context.Context, exec *gitcmd.Executor, opts BootstrapOptions) (BootstrapPlan, error) {
	if exec == nil {
		return BootstrapPlan{}, fmt.Errorf("git executor is nil")
	}
	if strings.TrimSpace(opts.Issuer) == "" {
		return BootstrapPlan{}, fmt.Errorf("--issuer is required")
	}
	g := newGitRepo(exec, opts.RepoPath)
	root, err := g.toplevel(ctx)
	if err != nil {
		return BootstrapPlan{}, err
	}
	g.dir = root
	plan, err := bootstrapSnapshot(ctx, g, opts)
	if err != nil {
		return BootstrapPlan{}, err
	}
	op, err := newOperationID()
	if err != nil {
		return BootstrapPlan{}, err
	}
	plan.Version, plan.Issuer, plan.OperationID = 1, strings.TrimSpace(opts.Issuer), op
	if opts.Expiry <= 0 || opts.Expiry > 15*time.Minute || opts.Expiry%time.Second != 0 {
		return BootstrapPlan{}, fmt.Errorf("expiry must be positive, whole seconds, and no greater than 15m")
	}
	issued := time.Now().UTC()
	plan.ExpiresAt = issued.Add(opts.Expiry).Format(time.RFC3339Nano)
	plan.IssuedAt = issued.Format(time.RFC3339Nano)
	plan.TTLSeconds = int64(opts.Expiry / time.Second)
	if plan.TTLSeconds <= 0 || plan.TTLSeconds > 900 {
		return BootstrapPlan{}, fmt.Errorf("expiry must be positive and no greater than 15m")
	}
	return plan, nil
}

// BootstrapApply recomputes a confirmation plan and pushes it with an exact
// lease after the caller supplies the reviewed canonical digest.
func BootstrapApply(ctx context.Context, exec *gitcmd.Executor, plan BootstrapPlan, repoPath, confirmation string) error {
	if exec == nil {
		return fmt.Errorf("git executor is nil")
	}
	if plan.Version != 1 || plan.OperationID == "" || plan.Issuer == "" || plan.IssuedAt == "" || plan.TTLSeconds <= 0 || plan.TTLSeconds > 900 {
		return fmt.Errorf("invalid bootstrap confirmation plan")
	}
	expectedDigest := BootstrapPlanDigest(plan)
	if expectedDigest == "" || confirmation != expectedDigest {
		return fmt.Errorf("confirmation does not match the canonical plan")
	}
	issued, err := time.Parse(time.RFC3339Nano, plan.IssuedAt)
	if err != nil {
		return fmt.Errorf("invalid issued_at")
	}
	exp, err := time.Parse(time.RFC3339Nano, plan.ExpiresAt)
	if err != nil || !exp.Equal(issued.Add(time.Duration(plan.TTLSeconds)*time.Second)) || issued.After(time.Now().UTC().Add(2*time.Minute)) || !time.Now().UTC().Before(exp) {
		return fmt.Errorf("bootstrap plan is expired")
	}
	g := newGitRepo(exec, repoPath)
	root, err := g.toplevel(ctx)
	if err != nil {
		return err
	}
	g.dir = root
	got, err := bootstrapSnapshot(ctx, g, BootstrapOptions{Branch: plan.SourceRef, Target: plan.TargetRef, Issuer: plan.Issuer})
	if err != nil {
		return err
	}
	if !sameBootstrapSnapshot(got, plan) {
		return fmt.Errorf("bootstrap state changed; re-plan")
	}
	if _, present, err := loadReadinessContract(ctx, g, plan.TargetSHA); err != nil {
		return err
	} else if present {
		return fmt.Errorf("target already declares readiness; plan is one-use")
	}
	res, err := g.run(ctx, "push", "--force-with-lease="+plan.DestinationRef+":"+plan.TargetSHA, plan.PushEndpoint, plan.SourceSHA+":"+plan.DestinationRef)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("bootstrap push failed: %s", strings.TrimSpace(res.Stderr))
	}
	return nil
}

func sameBootstrapSnapshot(a, b BootstrapPlan) bool {
	return a.Repository == b.Repository && a.Remote == b.Remote && a.TargetRef == b.TargetRef &&
		a.TargetSHA == b.TargetSHA && a.SourceRef == b.SourceRef && a.SourceSHA == b.SourceSHA &&
		a.ManifestPath == b.ManifestPath && a.ManifestOID == b.ManifestOID && a.RunnerPath == b.RunnerPath &&
		a.RunnerOID == b.RunnerOID && a.ReadinessTreeOID == b.ReadinessTreeOID &&
		a.ReadinessTreeDigest == b.ReadinessTreeDigest && a.PushEndpoint == b.PushEndpoint &&
		a.DestinationRef == b.DestinationRef
}

//nolint:gocyclo // Bootstrap validation is intentionally a linear fail-closed checklist.
func bootstrapSnapshot(ctx context.Context, g gitRepo, opts BootstrapOptions) (BootstrapPlan, error) {
	branch := strings.TrimSpace(opts.Branch)
	if branch == "" {
		var err error
		branch, err = g.currentBranch(ctx)
		if err != nil {
			return BootstrapPlan{}, err
		}
	}
	sha, ok, err := g.revParse(ctx, branch)
	if err != nil || !ok {
		return BootstrapPlan{}, fmt.Errorf("source branch not found: %s", branch)
	}
	remote, err := detectRemote(ctx, g, branch)
	if err != nil {
		return BootstrapPlan{}, err
	}
	if err := g.fetchPrune(ctx, remote); err != nil {
		return BootstrapPlan{}, err
	}
	endpoint, err := pushEndpoint(ctx, g, remote)
	if err != nil {
		return BootstrapPlan{}, err
	}
	target := strings.TrimSpace(opts.Target)
	if target == "" {
		return BootstrapPlan{}, fmt.Errorf("--target must name an explicit %s/<branch> ref", remote)
	}
	if !strings.HasPrefix(target, remote+"/") || strings.ContainsAny(target, " \t\r\n") {
		return BootstrapPlan{}, fmt.Errorf("--target must be a canonical remote-tracking ref")
	}
	branchName := strings.TrimPrefix(target, remote+"/")
	if strings.HasPrefix(target, "refs/") || branchName == "HEAD" || gitcmd.SanitizeBranchName(branchName) != nil {
		return BootstrapPlan{}, fmt.Errorf("--target must be exactly %s/<valid branch>", remote)
	}
	destination := "refs/heads/" + branchName
	remoteSHA, err := exactRemoteSHA(ctx, g, endpoint, destination)
	if err != nil {
		return BootstrapPlan{}, err
	}
	shaTarget, ok, err := g.revParse(ctx, target)
	if err != nil || !ok || shaTarget != remoteSHA {
		return BootstrapPlan{}, fmt.Errorf("target ref not found: %s", target)
	}
	if _, present, err := loadReadinessContract(ctx, g, shaTarget); err != nil {
		return BootstrapPlan{}, err
	} else if present {
		return BootstrapPlan{}, fmt.Errorf("target already declares readiness")
	}
	n, err := g.revCount(ctx, shaTarget+".."+sha)
	if err != nil || n != 1 {
		return BootstrapPlan{}, fmt.Errorf("source must be exactly one commit ahead of target")
	}
	anc, err := g.isAncestor(ctx, shaTarget, sha)
	if err != nil || !anc {
		return BootstrapPlan{}, fmt.Errorf("source is not a fast-forward of target")
	}
	if err := validateBootstrapDiff(ctx, g, shaTarget, sha); err != nil {
		return BootstrapPlan{}, err
	}
	contract, present, err := loadReadinessContract(ctx, g, sha)
	if err != nil || !present {
		return BootstrapPlan{}, fmt.Errorf("source readiness contract missing")
	}
	digest, err := readinessTreeDigest(ctx, g, sha)
	if err != nil {
		return BootstrapPlan{}, err
	}
	return BootstrapPlan{Repository: endpoint, Remote: remote, TargetRef: target, TargetSHA: shaTarget, SourceRef: branch, SourceSHA: sha, ManifestPath: contract.ManifestPath, ManifestOID: contract.ManifestOID, RunnerPath: contract.Decl.Runner, RunnerOID: contract.RunnerOID, ReadinessTreeOID: contract.TreeOID, ReadinessTreeDigest: digest, PushEndpoint: endpoint, DestinationRef: destination}, nil
}

func pushEndpoint(ctx context.Context, g gitRepo, remote string) (string, error) {
	res, err := g.run(ctx, "remote", "get-url", "--push", "--all", remote)
	if err != nil {
		return "", err
	}
	vals := splitNonEmpty(res.Stdout)
	if len(vals) != 1 {
		return "", fmt.Errorf("remote %s has ambiguous push endpoint", remote)
	}
	endpoint, err := canonicalPushEndpoint(strings.TrimSpace(vals[0]))
	if err != nil {
		return "", err
	}
	rewrites, err := g.run(ctx, "config", "--get-regexp", `^url\.`)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(rewrites.Stdout, "\n") {
		fields := strings.Fields(line)
		key := ""
		if len(fields) > 0 {
			key = strings.ToLower(fields[0])
		}
		isRewrite := strings.HasSuffix(key, ".insteadof") || strings.HasSuffix(key, ".pushinsteadof")
		if isRewrite && len(fields) >= 2 && strings.HasPrefix(endpoint, fields[len(fields)-1]) {
			return "", fmt.Errorf("effective push endpoint is subject to another URL rewrite")
		}
	}
	return endpoint, nil
}

func canonicalPushEndpoint(raw string) (string, error) {
	if raw == "" || strings.HasPrefix(raw, "-") || strings.IndexFunc(raw, func(r rune) bool { return r <= 0x20 || r == 0x7f }) >= 0 {
		return "", fmt.Errorf("push endpoint must be a credential-free URL")
	}
	if strings.Contains(raw, "@") && !strings.Contains(raw, "://") { // credential-free scp syntax: user@host:path
		at := strings.IndexByte(raw, '@')
		colon := strings.IndexByte(raw[at+1:], ':')
		if at <= 0 || colon <= 0 || at+1+colon+1 >= len(raw) || strings.Contains(raw[:at], ":") {
			return "", fmt.Errorf("push endpoint must be credential-free")
		}
		return raw, nil
	}
	u, err := url.Parse(raw)
	hasPassword := false
	if u != nil && u.User != nil {
		_, hasPassword = u.User.Password()
	}
	if err != nil || hasPassword || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("push endpoint must be a credential-free URL")
	}
	switch u.Scheme {
	case "": // local filesystem path
	case "file":
		if u.User != nil {
			return "", fmt.Errorf("push endpoint must be a credential-free URL")
		}
	case "http", "https":
		if u.Host == "" || u.User != nil {
			return "", fmt.Errorf("push endpoint must be a credential-free URL")
		}
	case "ssh", "git":
		if u.Host == "" {
			return "", fmt.Errorf("push endpoint must be a credential-free URL")
		}
	default:
		return "", fmt.Errorf("push endpoint must be a credential-free URL")
	}
	return u.String(), nil
}

func exactRemoteSHA(ctx context.Context, g gitRepo, endpoint, destination string) (string, error) {
	res, err := g.run(ctx, "ls-remote", endpoint, destination)
	if err != nil || res.ExitCode != 0 {
		return "", fmt.Errorf("resolve target from push endpoint: %w", err)
	}
	fields := strings.Fields(res.Stdout)
	if len(fields) < 2 || fields[1] != destination {
		return "", fmt.Errorf("target is missing at push endpoint")
	}
	return fields[0], nil
}

//nolint:gocyclo // Each accepted Git status, path, type, and mode is checked explicitly.
func validateBootstrapDiff(ctx context.Context, g gitRepo, target, source string) error {
	res, err := g.run(ctx, "diff", "--name-status", "--no-renames", "-z", target, source, "--")
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf("inspect bootstrap diff: %w", err)
	}
	parts := strings.Split(res.Stdout, "\x00")
	if len(parts) < 2 {
		return fmt.Errorf("bootstrap commit has no changes")
	}
	names := make([]string, 0)
	for i := 0; i+1 < len(parts); i += 2 {
		status, name := parts[i], parts[i+1]
		if status != "M" && status != "A" {
			return fmt.Errorf("bootstrap diff contains forbidden status %s", status)
		}
		if name != ".gz-git.yaml" && !strings.HasPrefix(name, ".gz-git/readiness/") {
			return fmt.Errorf("bootstrap commit changes forbidden path: %s", name)
		}
		if status == "A" && name == ".gz-git.yaml" {
			return fmt.Errorf("bootstrap requires existing .gz-git.yaml")
		}
		names = append(names, name)
	}
	entry, ok, err := g.treeEntry(ctx, source, ".gz-git.yaml")
	if err != nil || !ok || entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755") {
		return fmt.Errorf("bootstrap requires .gz-git.yaml")
	}
	oldEntry, oldOK, err := g.treeEntry(ctx, target, ".gz-git.yaml")
	if err != nil || !oldOK || oldEntry.Type != "blob" || oldEntry.Mode != entry.Mode {
		return fmt.Errorf(".gz-git.yaml mode must be unchanged")
	}
	for _, name := range names {
		e, ok, err := g.treeEntry(ctx, source, name)
		if err != nil || !ok || e.Type != "blob" {
			return fmt.Errorf("bootstrap path must be a regular blob: %s", name)
		}
		if strings.HasPrefix(name, ".gz-git/readiness/") && e.Mode != "100644" && e.Mode != "100755" {
			return fmt.Errorf("readiness path must be 100644 or 100755: %s", name)
		}
	}
	old, _, err := g.showFile(ctx, target, ".gz-git.yaml")
	if err != nil {
		return fmt.Errorf("target .gz-git.yaml required: %w", err)
	}
	neu, _, err := g.showFile(ctx, source, ".gz-git.yaml")
	if err != nil {
		return err
	}
	return onlyReadinessAddition(old, neu)
}

func onlyReadinessAddition(old, neu []byte) error {
	var a, b map[string]any
	if err := yaml.Unmarshal(old, &a); err != nil {
		return err
	}
	if err := yaml.Unmarshal(neu, &b); err != nil {
		return err
	}
	oldBranch, oldOK := a["branch"].(map[string]any)
	newBranch, newOK := b["branch"].(map[string]any)
	if !oldOK || !newOK {
		return fmt.Errorf("branch mapping is required")
	}
	if _, exists := oldBranch["readiness"]; exists {
		return fmt.Errorf("target already contains branch.readiness")
	}
	if _, exists := newBranch["readiness"]; !exists {
		return fmt.Errorf("source must add branch.readiness")
	}
	delete(newBranch, "readiness")
	equal, err := equalJSON(a, b)
	if err != nil {
		return fmt.Errorf("canonicalize config: %w", err)
	}
	if !equal {
		return fmt.Errorf(".gz-git.yaml contains changes beyond branch.readiness")
	}
	return nil
}

func equalJSON(a, b any) (bool, error) {
	x, err := json.Marshal(a)
	if err != nil {
		return false, err
	}
	y, err := json.Marshal(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(x, y), nil
}

func readinessTreeDigest(ctx context.Context, g gitRepo, sha string) (string, error) {
	out, err := g.output(ctx, "ls-tree", "-r", "--full-tree", sha, "--", ".gz-git/readiness")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(strings.SplitN(line, "\t", 2)[0])
		if len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			return "", fmt.Errorf("readiness tree contains non-regular or invalid-mode entry")
		}
	}
	sum := sha256.Sum256([]byte(out + "\n"))
	return hex.EncodeToString(sum[:]), nil
}

func newOperationID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate operation ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// BootstrapPlanDigest is the canonical value a human must explicitly confirm.
func BootstrapPlanDigest(p BootstrapPlan) string {
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// WriteBootstrapPlan writes a confirmation plan to stdout or a mode-0600 file.
func WriteBootstrapPlan(path string, p BootstrapPlan) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if path == "" {
		_, err = os.Stdout.Write(append(b, '\n'))
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// ReadBootstrapPlan decodes an exact V1 confirmation-plan JSON object.
func ReadBootstrapPlan(path string) (BootstrapPlan, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the operator explicitly selects the confirmation-plan file
	if err != nil {
		return BootstrapPlan{}, err
	}
	if err := uniquePlanJSON(b); err != nil {
		return BootstrapPlan{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		return BootstrapPlan{}, fmt.Errorf("invalid plan JSON")
	}
	if len(raw) != 20 {
		return BootstrapPlan{}, fmt.Errorf("plan has unknown or missing fields")
	}
	dec = json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var p BootstrapPlan
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	return p, nil
}

func uniquePlanJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return fmt.Errorf("plan must be a JSON object")
	}
	seen := map[string]bool{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid plan JSON")
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return fmt.Errorf("duplicate plan field")
		}
		seen[name] = true
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return fmt.Errorf("invalid plan JSON")
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("invalid plan JSON")
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("trailing plan JSON")
	}
	return nil
}
