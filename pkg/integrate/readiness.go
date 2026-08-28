// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

const (
	readinessTimeout     = 15 * time.Minute
	readinessMaxIO       = 1 << 20
	readinessMaxSummary  = 4096
	readinessMaxManifest = 64 << 10
	readinessWaitDelay   = 2 * time.Second
)

type readinessContract struct {
	Decl         config.Readiness
	ManifestPath string
	ManifestOID  string
	Digest       string
	RunnerOID    string
	TreeOID      string
}

type readinessResult struct {
	Version int    `json:"version"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

func loadReadinessContract(ctx context.Context, g gitRepo, sha string) (readinessContract, bool, error) {
	for _, name := range []string{".gz-git.yaml", ".gz-git.yml", ".gz-git.json"} {
		manifest, present, err := g.treeEntry(ctx, sha, name)
		if err != nil {
			return readinessContract{}, false, err
		}
		if !present {
			continue
		}
		return loadReadinessManifest(ctx, g, sha, name, manifest)
	}
	return readinessContract{}, false, nil
}

func loadReadinessManifest(ctx context.Context, g gitRepo, sha, name string, manifest treeEntry) (readinessContract, bool, error) {
	if manifest.Type != "blob" || (manifest.Mode != "100644" && manifest.Mode != "100755") {
		return readinessContract{}, false, nil
	}
	data, present, err := g.showFile(ctx, sha, name)
	if err != nil {
		return readinessContract{}, false, err
	}
	if !present {
		return readinessContract{}, false, nil
	}
	decl, present, err := config.ParseReadinessDocument(data, strings.HasSuffix(name, ".json"))
	if err != nil {
		return readinessContract{}, false, fmt.Errorf("%s: %w", name, err)
	}
	if !present {
		return readinessContract{}, false, nil
	}
	size, err := g.objectSize(ctx, manifest.OID)
	if err != nil {
		return readinessContract{}, false, err
	}
	if size > readinessMaxManifest {
		return readinessContract{}, false, fmt.Errorf("%s exceeds readiness manifest limit", name)
	}
	digest, err := config.ReadinessDigest(decl)
	if err != nil {
		return readinessContract{}, false, err
	}
	runner, ok, err := g.treeEntry(ctx, sha, decl.Runner)
	if err != nil {
		return readinessContract{}, false, err
	}
	if !ok || runner.Type != "blob" || runner.Mode != "100755" {
		return readinessContract{}, false, fmt.Errorf("readiness runner %s must be an executable regular blob (mode 100755)", decl.Runner)
	}
	tree, ok, err := g.treeEntry(ctx, sha, ".gz-git/readiness")
	if err != nil {
		return readinessContract{}, false, err
	}
	if !ok || tree.Type != "tree" {
		return readinessContract{}, false, fmt.Errorf(".gz-git/readiness is missing or not a tree")
	}
	return readinessContract{Decl: decl, ManifestPath: name, ManifestOID: manifest.OID, Digest: digest, RunnerOID: runner.OID, TreeOID: tree.OID}, true, nil
}

func sameReadinessContract(a, b readinessContract) bool {
	return a.Digest == b.Digest && a.TreeOID == b.TreeOID
}

func checkReadinessContract(ctx context.Context, g gitRepo, plan TargetPlan, report *CheckReport) CheckItem {
	target, targetPresent, err := loadReadinessContract(ctx, g, plan.TargetSHA)
	if err != nil {
		return CheckItem{Name: "readiness contract", Status: checkFail, Detail: err.Error()}
	}
	source, sourcePresent, err := loadReadinessContract(ctx, g, plan.BranchSHA)
	if err != nil {
		return CheckItem{Name: "readiness contract", Status: checkFail, Detail: err.Error()}
	}
	if !targetPresent && !sourcePresent {
		report.GateMode = "legacy-make"
		return CheckItem{Name: "readiness contract", Status: checkWarn, Detail: "legacy head-owned gate"}
	}
	if !targetPresent {
		return CheckItem{Name: "readiness contract", Status: checkFail, Detail: "bootstrap required: target has no readiness contract"}
	}
	if !sourcePresent {
		return CheckItem{Name: "readiness contract", Status: checkFail, Detail: "source is missing target readiness contract"}
	}
	if !sameReadinessContract(target, source) {
		return CheckItem{Name: "readiness contract", Status: checkFail, Detail: "readiness contract changed between target and source"}
	}
	if !readinessRunnerSupported() {
		return CheckItem{Name: "readiness contract", Status: checkFail, Detail: "measurement unavailable: contract runners are unsupported on Windows"}
	}
	report.GateMode, report.ManifestPath, report.ContractDigest, report.ManifestOID = "contract-v1", target.ManifestPath, target.Digest, target.ManifestOID
	report.RunnerPath, report.RunnerOID, report.ReadinessTreeOID = target.Decl.Runner, target.RunnerOID, target.TreeOID
	result, duration, err := runContract(ctx, g, plan, target)
	report.ReadinessStatus, report.ReadinessDuration = result.Status, duration
	if err != nil {
		return CheckItem{Name: "readiness contract", Status: checkFail, Detail: "measurement unavailable: " + err.Error()}
	}
	if result.Status == "ready" {
		return CheckItem{Name: "readiness contract", Status: checkPass, Detail: result.Summary}
	}
	if result.Status == "unavailable" {
		return CheckItem{Name: "readiness contract", Status: checkFail, Detail: "measurement unavailable: " + result.Summary}
	}
	return CheckItem{Name: "readiness contract", Status: checkFail, Detail: result.Summary}
}

func runContract(ctx context.Context, g gitRepo, plan TargetPlan, contract readinessContract) (result readinessResult, duration time.Duration, err error) {
	return runContractWithTimeout(ctx, g, plan, contract, readinessTimeout)
}

func runContractWithTimeout(ctx context.Context, g gitRepo, plan TargetPlan, contract readinessContract, timeout time.Duration) (result readinessResult, duration time.Duration, err error) {
	root, err := os.MkdirTemp("", "gz-git-readiness-")
	if err != nil {
		return readinessResult{Status: "unavailable"}, 0, fmt.Errorf("create isolated worktrees: %w", err)
	}
	defer func() {
		if err != nil {
			result.Status = "unavailable"
		}
	}()
	defer func() {
		if removeErr := os.RemoveAll(root); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("cleanup readiness workspace: %w", removeErr))
		}
	}()
	targetDir, sourceDir := filepath.Join(root, "target"), filepath.Join(root, "source")
	if err := g.worktreeAddDetach(ctx, targetDir, plan.TargetSHA); err != nil {
		return readinessResult{Status: "unavailable"}, 0, err
	}
	defer func() { err = mergeCleanupError(ctx, err, g, targetDir) }()
	if err := g.worktreeAddDetach(ctx, sourceDir, plan.BranchSHA); err != nil {
		return readinessResult{Status: "unavailable"}, 0, err
	}
	defer func() { err = mergeCleanupError(ctx, err, g, sourceDir) }()
	runner := filepath.Join(targetDir, filepath.FromSlash(contract.Decl.Runner))
	started := time.Now()
	result, err = executeReadinessWithTimeout(ctx, runner, targetDir, sourceDir, plan.BranchSHA, plan.TargetSHA, timeout)
	return result, time.Since(started), err
}

func mergeCleanupError(parent context.Context, current error, g gitRepo, dir string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), 30*time.Second)
	defer cancel()
	if err := g.worktreeRemoveForce(cleanupCtx, dir); err != nil {
		removeErr := fmt.Errorf("cleanup readiness worktree: %w", err)
		fsErr := os.RemoveAll(dir)
		return errors.Join(current, removeErr, fsErr)
	}
	return current
}

func executeReadinessWithTimeout(parent context.Context, runner, targetDir, sourceDir, sourceSHA, targetSHA string, timeout time.Duration) (readinessResult, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, runner, "--source-dir", sourceDir, "--source-sha", sourceSHA, "--target-sha", targetSHA, "--result-format", "json-v1") // #nosec G204 -- runner is target-owned, validated Git blob
	cmd.Dir = targetDir
	cmd.Env = readinessEnv(os.Environ())
	configureReadinessProcess(cmd)
	cmd.Cancel = func() error { return killReadinessProcess(cmd.Process) }
	cmd.WaitDelay = readinessWaitDelay
	var stdout, stderr limitedBuffer
	stdout.limit, stderr.limit = readinessMaxIO, readinessMaxIO
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return readinessResult{Status: "unavailable"}, fmt.Errorf("runner timed out")
	}
	if ctx.Err() != nil {
		return readinessResult{Status: "unavailable"}, ctx.Err()
	}
	if stdout.exceeded || stderr.exceeded {
		return readinessResult{Status: "unavailable"}, fmt.Errorf("runner output exceeded limit")
	}
	if runErr != nil {
		return readinessResult{Status: "unavailable"}, fmt.Errorf("runner failed: %s", boundedDetail(stderr.String()))
	}
	result, err := parseReadinessResult(stdout.Bytes())
	if err != nil {
		return readinessResult{Status: "unavailable"}, err
	}
	return result, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		b.exceeded = true
		n := b.limit - b.Len()
		if n > 0 {
			_, _ = b.Buffer.Write(p[:n])
		}
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

func parseReadinessResult(data []byte) (readinessResult, error) {
	if len(data) == 0 || len(data) > readinessMaxIO || !utf8.Valid(data) || bytes.IndexFunc(data, func(r rune) bool { return r < 0x20 && r != '\n' && r != '\r' && r != '\t' }) >= 0 {
		return readinessResult{}, fmt.Errorf("runner emitted invalid JSON")
	}
	raw, err := decodeReadinessResultObject(data)
	if err != nil {
		return readinessResult{}, err
	}
	if len(raw) != 3 || raw["version"] == nil || raw["status"] == nil || raw["summary"] == nil {
		return readinessResult{}, fmt.Errorf("runner result has unknown or missing fields")
	}
	var result readinessResult
	if err := json.Unmarshal(data, &result); err != nil {
		return readinessResult{}, fmt.Errorf("decode runner result: %w", err)
	}
	if result.Version != 1 || (result.Status != "ready" && result.Status != "not_ready" && result.Status != "unavailable") || !validSummary(result.Summary) {
		return readinessResult{}, fmt.Errorf("runner result violates json-v1")
	}
	return result, nil
}

func decodeReadinessResultObject(data []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("runner result is not a JSON object")
	}
	raw := make(map[string]json.RawMessage, 3)
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("runner result is not valid JSON: %w", err)
		}
		name, ok := key.(string)
		if !ok || raw[name] != nil {
			return nil, fmt.Errorf("runner result has duplicate or invalid fields")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("runner result is not valid JSON: %w", err)
		}
		raw[name] = value
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("runner result is not valid JSON: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return nil, fmt.Errorf("runner emitted trailing data")
	}
	return raw, nil
}

func validSummary(s string) bool {
	if strings.TrimSpace(s) == "" || len(s) > readinessMaxSummary {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func boundedDetail(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, ansiEscape.ReplaceAllString(s, ""))
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "non-zero exit"
	}
	return string([]rune(s)[:min(len([]rune(s)), 512)])
}

func readinessEnv(env []string) []string {
	blocked := map[string]bool{"BASH_ENV": true, "ENV": true, "CDPATH": true, "PROMPT_COMMAND": true, "GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true}
	out := make([]string, 0, len(env)+2)
	for _, item := range env {
		key, _, found := strings.Cut(item, "=")
		if found && !blocked[key] && key != "LC_ALL" && key != "LANG" {
			out = append(out, item)
		}
	}
	return append(out, "LC_ALL=C", "LANG=C")
}
