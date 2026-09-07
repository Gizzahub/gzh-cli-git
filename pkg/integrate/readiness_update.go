package integrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

// ReadinessUpdatePlan is the one-use, human-confirmed transaction for changing
// a target-owned readiness V1 contract. It intentionally binds both sides of
// the contract, rather than treating a changed runner as an ordinary run.
type ReadinessUpdatePlan struct {
	Version                int64  `json:"version"`
	OperationID            string `json:"operation_id"`
	Issuer                 string `json:"issuer"`
	ExpiresAt              string `json:"expires_at"`
	Repository             string `json:"repository"`
	Remote                 string `json:"remote"`
	TargetRef              string `json:"target_ref"`
	TargetSHA              string `json:"target_sha"`
	SourceRef              string `json:"source_ref"`
	SourceSHA              string `json:"source_sha"`
	PushEndpoint           string `json:"push_endpoint"`
	DestinationRef         string `json:"destination_ref"`
	OperationRef           string `json:"operation_ref"`
	IssuedAt               string `json:"issued_at"`
	TTLSeconds             int64  `json:"ttl_seconds"`
	OldManifestPath        string `json:"old_manifest_path"`
	OldManifestOID         string `json:"old_manifest_oid"`
	OldRunnerPath          string `json:"old_runner_path"`
	OldRunnerOID           string `json:"old_runner_oid"`
	OldReadinessTreeOID    string `json:"old_readiness_tree_oid"`
	OldReadinessTreeDigest string `json:"old_readiness_tree_digest"`
	OldReadinessTreePath   string `json:"old_readiness_tree_path"`
	OldContractDigest      string `json:"old_contract_digest"`
	NewManifestPath        string `json:"new_manifest_path"`
	NewManifestOID         string `json:"new_manifest_oid"`
	NewRunnerPath          string `json:"new_runner_path"`
	NewRunnerOID           string `json:"new_runner_oid"`
	NewReadinessTreeOID    string `json:"new_readiness_tree_oid"`
	NewReadinessTreeDigest string `json:"new_readiness_tree_digest"`
	NewReadinessTreePath   string `json:"new_readiness_tree_path"`
	NewContractDigest      string `json:"new_contract_digest"`
}

// ReadinessUpdateOptions selects the source and target for an update plan.
type ReadinessUpdateOptions struct {
	RepoPath, Branch, Target, Issuer string
	Expiry                           time.Duration
}

// ReadinessUpdatePlanFor validates and snapshots a one-commit contract update.
func ReadinessUpdatePlanFor(ctx context.Context, exec *gitcmd.Executor, opts ReadinessUpdateOptions) (ReadinessUpdatePlan, error) {
	if exec == nil {
		return ReadinessUpdatePlan{}, fmt.Errorf("git executor is nil")
	}
	if strings.TrimSpace(opts.Issuer) == "" {
		return ReadinessUpdatePlan{}, fmt.Errorf("--issuer is required")
	}
	if opts.Expiry <= 0 || opts.Expiry > 15*time.Minute || opts.Expiry%time.Second != 0 {
		return ReadinessUpdatePlan{}, fmt.Errorf("expiry must be positive, whole seconds, and no greater than 15m")
	}
	g := newGitRepo(exec, opts.RepoPath)
	root, err := g.toplevel(ctx)
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	g.dir = root
	p, err := readinessUpdateSnapshot(ctx, g, opts)
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	op, err := newOperationID()
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	now := time.Now().UTC()
	p.Version, p.OperationID, p.Issuer = 1, op, strings.TrimSpace(opts.Issuer)
	p.OperationRef = readinessUpdateOperationRef(op)
	p.IssuedAt, p.ExpiresAt, p.TTLSeconds = now.Format(time.RFC3339Nano), now.Add(opts.Expiry).Format(time.RFC3339Nano), int64(opts.Expiry/time.Second)
	if err := ensureOperationAbsent(ctx, g, p.PushEndpoint, p.OperationRef); err != nil {
		return ReadinessUpdatePlan{}, err
	}
	return p, nil
}

// ReadinessUpdateApply revalidates and leases the reviewed update plan. Callers
// must enforce the human authorization boundary; the CLI requires a TTY and the
// downstream hook independently rejects unapproved direct invocations.
func ReadinessUpdateApply(ctx context.Context, exec *gitcmd.Executor, plan ReadinessUpdatePlan, repoPath, confirmation string) error {
	if exec == nil {
		return fmt.Errorf("git executor is nil")
	}
	if !validReadinessUpdatePlan(plan) {
		return fmt.Errorf("invalid readiness update confirmation plan")
	}
	if confirmation == "" || confirmation != ReadinessUpdatePlanDigest(plan) {
		return fmt.Errorf("confirmation does not match the canonical plan")
	}
	issued, err := time.Parse(time.RFC3339Nano, plan.IssuedAt)
	if err != nil {
		return fmt.Errorf("invalid issued_at")
	}
	expires, err := time.Parse(time.RFC3339Nano, plan.ExpiresAt)
	if err != nil || !expires.Equal(issued.Add(time.Duration(plan.TTLSeconds)*time.Second)) || issued.After(time.Now().UTC().Add(2*time.Minute)) || !time.Now().UTC().Before(expires) {
		return fmt.Errorf("readiness update plan is expired")
	}
	g := newGitRepo(exec, repoPath)
	root, err := g.toplevel(ctx)
	if err != nil {
		return err
	}
	g.dir = root
	got, err := readinessUpdateSnapshot(ctx, g, ReadinessUpdateOptions{Branch: plan.SourceRef, Target: plan.TargetRef, Issuer: plan.Issuer})
	if err != nil {
		return err
	}
	if !sameReadinessUpdateSnapshot(got, plan) {
		return fmt.Errorf("readiness update state changed; re-plan")
	}
	if err := ensureOperationAbsent(ctx, g, plan.PushEndpoint, plan.OperationRef); err != nil {
		return err
	}
	// A current target with the old contract is required; this also makes plans one-use.
	current, present, err := updateReadinessContract(ctx, g, plan.TargetSHA)
	if err != nil {
		return err
	}
	if !present || current.ManifestOID != plan.OldManifestOID || current.RunnerOID != plan.OldRunnerOID || current.TreeOID != plan.OldReadinessTreeOID || current.Digest != plan.OldContractDigest {
		return fmt.Errorf("target readiness contract changed; plan is one-use")
	}
	res, err := g.run(ctx, "push", "--atomic", "--force-with-lease="+plan.DestinationRef+":"+plan.TargetSHA, "--force-with-lease="+plan.OperationRef+":", plan.PushEndpoint, plan.SourceSHA+":"+plan.DestinationRef, plan.SourceSHA+":"+plan.OperationRef)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("atomic readiness update push failed (remote must support atomic push): %s", strings.TrimSpace(res.Stderr))
	}
	return nil
}

func readinessUpdateSnapshot(ctx context.Context, g gitRepo, opts ReadinessUpdateOptions) (ReadinessUpdatePlan, error) { //nolint:gocyclo // fail-closed transaction validation is intentionally linear
	branch := strings.TrimSpace(opts.Branch)
	var err error
	if branch == "" {
		branch, err = g.currentBranch(ctx)
		if err != nil {
			return ReadinessUpdatePlan{}, err
		}
	}
	source, ok, err := g.revParse(ctx, branch)
	if err != nil || !ok {
		return ReadinessUpdatePlan{}, fmt.Errorf("source branch not found: %s", branch)
	}
	remote, err := detectRemote(ctx, g, branch)
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	if err := g.fetchPrune(ctx, remote); err != nil {
		return ReadinessUpdatePlan{}, err
	}
	endpoint, err := pushEndpoint(ctx, g, remote)
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	target := strings.TrimSpace(opts.Target)
	if target == "" || !strings.HasPrefix(target, remote+"/") || strings.ContainsAny(target, " \t\r\n") {
		return ReadinessUpdatePlan{}, fmt.Errorf("--target must be a canonical remote-tracking ref")
	}
	name := strings.TrimPrefix(target, remote+"/")
	if strings.HasPrefix(target, "refs/") || name == "HEAD" || gitcmd.SanitizeBranchName(name) != nil {
		return ReadinessUpdatePlan{}, fmt.Errorf("--target must be exactly %s/<valid branch>", remote)
	}
	destination := "refs/heads/" + name
	remoteSHA, err := exactRemoteSHA(ctx, g, endpoint, destination)
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	targetSHA, ok, err := g.revParse(ctx, target)
	if err != nil || !ok || targetSHA != remoteSHA {
		return ReadinessUpdatePlan{}, fmt.Errorf("target ref not found: %s", target)
	}
	old, oldOK, err := updateReadinessContract(ctx, g, targetSHA)
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	if !oldOK {
		return ReadinessUpdatePlan{}, fmt.Errorf("target readiness contract missing")
	}
	if n, err := g.revCount(ctx, targetSHA+".."+source); err != nil || n != 1 {
		return ReadinessUpdatePlan{}, fmt.Errorf("source must be exactly one commit ahead of target")
	}
	ancestor, err := g.isAncestor(ctx, targetSHA, source)
	if err != nil || !ancestor {
		return ReadinessUpdatePlan{}, fmt.Errorf("source is not a fast-forward of target")
	}
	neu, newOK, err := updateReadinessContract(ctx, g, source)
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	if !newOK {
		return ReadinessUpdatePlan{}, fmt.Errorf("source readiness contract missing")
	}
	if old.ManifestPath != neu.ManifestPath {
		return ReadinessUpdatePlan{}, fmt.Errorf("readiness manifest path migration is not allowed")
	}
	if err := validateReadinessUpdateDiff(ctx, g, targetSHA, source, old.ManifestPath); err != nil {
		return ReadinessUpdatePlan{}, err
	}
	oldDigest, err := readinessTreeDigest(ctx, g, targetSHA)
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	newDigest, err := readinessTreeDigest(ctx, g, source)
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	if old.ManifestOID == neu.ManifestOID && old.RunnerOID == neu.RunnerOID && old.TreeOID == neu.TreeOID && oldDigest == newDigest {
		return ReadinessUpdatePlan{}, fmt.Errorf("readiness contract must change")
	}
	return ReadinessUpdatePlan{
		Repository: endpoint, Remote: remote, TargetRef: target, TargetSHA: targetSHA, SourceRef: branch, SourceSHA: source, PushEndpoint: endpoint, DestinationRef: destination,
		OldManifestPath: old.ManifestPath, OldManifestOID: old.ManifestOID, OldRunnerPath: old.Decl.Runner, OldRunnerOID: old.RunnerOID, OldReadinessTreeOID: old.TreeOID, OldReadinessTreeDigest: oldDigest, OldReadinessTreePath: ".gz-git/readiness", OldContractDigest: old.Digest,
		NewManifestPath: neu.ManifestPath, NewManifestOID: neu.ManifestOID, NewRunnerPath: neu.Decl.Runner, NewRunnerOID: neu.RunnerOID, NewReadinessTreeOID: neu.TreeOID, NewReadinessTreeDigest: newDigest, NewReadinessTreePath: ".gz-git/readiness", NewContractDigest: neu.Digest,
	}, nil
}

// updateReadinessContract is intentionally stricter than normal readiness
// loading: a policy transaction must not silently select one of two manifests.
func updateReadinessContract(ctx context.Context, g gitRepo, sha string) (readinessContract, bool, error) {
	var name string
	var entry treeEntry
	for _, candidate := range []string{".gz-git.yaml", ".gz-git.yml", ".gz-git.json"} {
		e, ok, err := g.treeEntry(ctx, sha, candidate)
		if err != nil {
			return readinessContract{}, false, err
		}
		if !ok {
			continue
		}
		if name != "" {
			return readinessContract{}, false, fmt.Errorf("multiple readiness manifests are not allowed")
		}
		name, entry = candidate, e
	}
	if name == "" {
		return readinessContract{}, false, nil
	}
	size, err := g.objectSize(ctx, entry.OID)
	if err != nil {
		return readinessContract{}, false, err
	}
	if size > readinessMaxManifest {
		return readinessContract{}, false, fmt.Errorf("%s exceeds readiness manifest limit", name)
	}
	data, present, err := g.showFile(ctx, sha, name)
	if err != nil || !present {
		return readinessContract{}, false, err
	}
	if err := validateUniqueManifest(data, strings.HasSuffix(name, ".json")); err != nil {
		return readinessContract{}, false, fmt.Errorf("%s: %w", name, err)
	}
	return loadReadinessManifest(ctx, g, sha, name, entry)
}

func validateReadinessUpdateDiff(ctx context.Context, g gitRepo, target, source, manifest string) error {
	res, err := g.run(ctx, "diff", "--name-status", "--no-renames", "-z", target, source, "--")
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf("inspect readiness update diff: %w", err)
	}
	parts := strings.Split(res.Stdout, "\x00")
	changed := false
	for i := 0; i+1 < len(parts); i += 2 {
		status, name := parts[i], parts[i+1]
		if status == "" {
			continue
		}
		changed = true
		if status != "A" && status != "M" && status != "D" {
			return fmt.Errorf("readiness update diff contains forbidden status %s", status)
		}
		if name != manifest && !strings.HasPrefix(name, ".gz-git/readiness/") {
			return fmt.Errorf("readiness update changes forbidden path: %s", name)
		}
	}
	if !changed {
		return fmt.Errorf("readiness update commit has no changes")
	}
	// The final manifest and every final readiness entry must be ordinary blobs.
	e, ok, err := g.treeEntry(ctx, source, manifest)
	if err != nil || !ok || e.Type != "blob" || (e.Mode != "100644" && e.Mode != "100755") {
		return fmt.Errorf("readiness manifest must be a regular blob with mode 100644 or 100755")
	}
	if _, err := readinessTreeDigest(ctx, g, source); err != nil {
		return err
	}
	return nil
}

func sameReadinessUpdateSnapshot(a, b ReadinessUpdatePlan) bool {
	return a.Repository == b.Repository && a.Remote == b.Remote && a.TargetRef == b.TargetRef && a.TargetSHA == b.TargetSHA && a.SourceRef == b.SourceRef && a.SourceSHA == b.SourceSHA && a.PushEndpoint == b.PushEndpoint && a.DestinationRef == b.DestinationRef &&
		a.OldManifestPath == b.OldManifestPath && a.OldManifestOID == b.OldManifestOID && a.OldRunnerPath == b.OldRunnerPath && a.OldRunnerOID == b.OldRunnerOID && a.OldReadinessTreeOID == b.OldReadinessTreeOID && a.OldReadinessTreeDigest == b.OldReadinessTreeDigest && a.OldReadinessTreePath == b.OldReadinessTreePath && a.OldContractDigest == b.OldContractDigest &&
		a.NewManifestPath == b.NewManifestPath && a.NewManifestOID == b.NewManifestOID && a.NewRunnerPath == b.NewRunnerPath && a.NewRunnerOID == b.NewRunnerOID && a.NewReadinessTreeOID == b.NewReadinessTreeOID && a.NewReadinessTreeDigest == b.NewReadinessTreeDigest && a.NewReadinessTreePath == b.NewReadinessTreePath && a.NewContractDigest == b.NewContractDigest
}

func validReadinessUpdatePlan(p ReadinessUpdatePlan) bool {
	return p.Version == 1 && validOperationRef(p.OperationID, p.OperationRef) && p.Issuer != "" && p.IssuedAt != "" && p.ExpiresAt != "" && p.TTLSeconds > 0 && p.TTLSeconds <= 900 && p.OldReadinessTreePath == ".gz-git/readiness" && p.NewReadinessTreePath == ".gz-git/readiness"
}

const readinessUpdateOperationPrefix = "refs/gz-git/readiness-update/operations/"

func readinessUpdateOperationRef(operationID string) string {
	return readinessUpdateOperationPrefix + operationID
}

func validOperationRef(operationID, ref string) bool {
	if len(operationID) != 32 || ref != readinessUpdateOperationRef(operationID) {
		return false
	}
	for _, r := range operationID {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func ensureOperationAbsent(ctx context.Context, g gitRepo, endpoint, ref string) error {
	res, err := g.run(ctx, "ls-remote", endpoint, ref)
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf("resolve readiness update operation marker: %w", err)
	}
	if strings.TrimSpace(res.Stdout) != "" {
		return fmt.Errorf("readiness update operation is already consumed")
	}
	return nil
}

// validateUniqueManifest rejects duplicate mapping keys anywhere in a selected
// manifest. Config readiness parsing is deliberately narrower, but a policy
// transaction must never preserve ambiguous non-readiness configuration either.
func validateUniqueManifest(data []byte, isJSON bool) error {
	if isJSON {
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := consumeUniqueJSON(dec); err != nil {
			return err
		}
		if dec.Decode(&struct{}{}) != io.EOF {
			return fmt.Errorf("trailing JSON")
		}
		return nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}
	return uniqueYAMLNode(&node)
}

func consumeUniqueJSON(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON")
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	if delim == '{' {
		return consumeUniqueJSONObject(dec)
	}
	if delim == '[' {
		return consumeUniqueJSONArray(dec)
	}
	return nil
}

func consumeUniqueJSONObject(dec *json.Decoder) error {
	seen := map[string]bool{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON")
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return fmt.Errorf("duplicate JSON key %q", name)
		}
		seen[name] = true
		if err := consumeUniqueJSON(dec); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("invalid JSON")
	}
	return nil
}

func consumeUniqueJSONArray(dec *json.Decoder) error {
	for dec.More() {
		if err := consumeUniqueJSON(dec); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("invalid JSON")
	}
	return nil
}

func uniqueYAMLNode(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		seen := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if seen[key] {
				return fmt.Errorf("duplicate YAML key %q", key)
			}
			seen[key] = true
			if err := uniqueYAMLNode(node.Content[i+1]); err != nil {
				return err
			}
		}
		return nil
	case yaml.DocumentNode, yaml.SequenceNode, yaml.AliasNode:
		for _, child := range node.Content {
			if err := uniqueYAMLNode(child); err != nil {
				return err
			}
		}
		return nil
	case yaml.ScalarNode:
		return nil
	default:
		return fmt.Errorf("unknown YAML node kind")
	}
}

// ReadinessUpdatePlanDigest returns the canonical human-confirmation digest.
func ReadinessUpdatePlanDigest(p ReadinessUpdatePlan) string {
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// WriteReadinessUpdatePlan writes a plan to stdout or a mode-0600 file.
func WriteReadinessUpdatePlan(path string, p ReadinessUpdatePlan) error {
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

// ReadReadinessUpdatePlan decodes an exact V1 plan object.
func ReadReadinessUpdatePlan(path string) (ReadinessUpdatePlan, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the operator explicitly selects the confirmation-plan file
	if err != nil {
		return ReadinessUpdatePlan{}, err
	}
	if err := uniquePlanJSON(b); err != nil {
		return ReadinessUpdatePlan{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil || dec.Decode(&struct{}{}) != io.EOF {
		return ReadinessUpdatePlan{}, fmt.Errorf("invalid plan JSON")
	}
	if len(raw) != 31 {
		return ReadinessUpdatePlan{}, fmt.Errorf("plan has unknown or missing fields")
	}
	dec = json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var p ReadinessUpdatePlan
	if err := dec.Decode(&p); err != nil {
		return p, err
	}
	return p, nil
}
