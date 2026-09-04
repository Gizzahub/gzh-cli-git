// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package contextref

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var ceEnvAllow = []string{
	"HOME", "LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES",
	"TZ", "TMPDIR", "TMP", "TEMP", "USER", "LOGNAME",
	"XDG_RUNTIME_DIR", "XDG_CACHE_HOME", "XDG_CONFIG_HOME",
	"PATH", "SYSTEMROOT", "COMSPEC", "PATHEXT",
}

type ceVersionReport struct {
	Version      string   `json:"version"`
	Revision     string   `json:"revision"`
	Capabilities []string `json:"capabilities"`
}

type ceDoctorReport struct {
	CapabilityID      string       `json:"capabilityId"`      //nolint:tagliatelle // CE v2
	SchemaVersion     int          `json:"schemaVersion"`     //nolint:tagliatelle // CE v2
	ObservationSource string       `json:"observationSource"` //nolint:tagliatelle // CE v2
	SymlinkSafe       bool         `json:"symlinkSafe"`       //nolint:tagliatelle // CE v2
	RepositoryID      string       `json:"repositoryId"`      //nolint:tagliatelle // CE v2
	CommonGitDir      string       `json:"commonGitDir"`      //nolint:tagliatelle // CE v2
	Worktrees         []ceWorktree `json:"worktrees"`
	Gates             []ceGate     `json:"gates"`
	Status            string       `json:"status"`
	Remediation       string       `json:"remediation"`
	BuildRevision     string       `json:"buildRevision"` //nolint:tagliatelle // CE v2
}

type ceWorktree struct {
	Path      string      `json:"path"`
	HooksPath ceHooksPath `json:"hooksPath"` //nolint:tagliatelle // CE v2
}

type ceHooksPath struct {
	Origin   string `json:"origin,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Value    string `json:"value,omitempty"`
	Resolved string `json:"resolved,omitempty"`
	Set      bool   `json:"set"`
}

type ceGate struct {
	Worktree        string        `json:"worktree"`
	Declaration     ceDeclaration `json:"declaration"`
	Exists          bool          `json:"exists"`
	Symlink         bool          `json:"symlink"`
	RegularFile     bool          `json:"regularFile"`       //nolint:tagliatelle // CE v2
	GitMode         string        `json:"gitMode,omitempty"` //nolint:tagliatelle // CE v2
	Dirty           bool          `json:"dirty"`
	SourceRevision  string        `json:"sourceRevision,omitempty"`  //nolint:tagliatelle // CE v2
	ObservedBlob    string        `json:"observedBlob,omitempty"`    //nolint:tagliatelle // CE v2
	ObservedDigest  string        `json:"observedDigest,omitempty"`  //nolint:tagliatelle // CE v2
	ObservedVersion int           `json:"observedVersion,omitempty"` //nolint:tagliatelle // CE v2
	Executable      bool          `json:"executable"`
	Status          string        `json:"status"`
	Remediation     string        `json:"remediation"`
}

type ceDeclaration struct {
	Event          string `json:"event"`
	ID             string `json:"id"`
	HookPath       string `json:"hookPath"`       //nolint:tagliatelle // CE v2
	PayloadVersion int    `json:"payloadVersion"` //nolint:tagliatelle // CE v2
	PayloadDigest  string `json:"payloadDigest"`  //nolint:tagliatelle // CE v2
}

func observeCE(ctx context.Context, opts Options, worktree string) Component {
	if opts.CE == nil {
		return Component{ComponentOutcome: OutcomeAbsent}
	}
	ident, digest, err := inspectExecutable(opts.CE.Path)
	if err != nil {
		return faultComponent(err.Error())
	}
	if digest != opts.CE.Digest {
		return faultComponent("ce digest mismatch")
	}
	env := filteredEnv()
	ver, err := invokeCE(ctx, opts, worktree, env, []string{"version", "--json"})
	if err != nil {
		return faultComponent(err.Error())
	}
	if ver.exit != 0 {
		return faultComponent("ce handshake failed")
	}
	if err := verifyHandshake(ver.stdout, opts.CE.CapabilityID); err != nil {
		return faultComponent(err.Error())
	}
	doc, err := invokeCE(ctx, opts, worktree, env, []string{
		"task", "doctor", "--json", "--capability", opts.CE.CapabilityID,
	})
	if err != nil {
		return faultComponent(err.Error())
	}
	after, afterDigest, err := inspectExecutable(opts.CE.Path)
	if err != nil {
		return faultComponent(err.Error())
	}
	if after != ident || afterDigest != digest {
		return faultComponent("ce identity changed")
	}
	return mapDoctorResult(doc)
}

func inspectExecutable(path string) (fileIdent, string, error) {
	info, err := lstatNoFollow(path)
	if err != nil {
		return fileIdent{}, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fileIdent{}, "", errSymlink
	}
	if !info.Mode().IsRegular() {
		return fileIdent{}, "", errNonRegular
	}
	f, err := openExecNoFollow(path)
	if err != nil {
		return fileIdent{}, "", err
	}
	defer func() { _ = f.Close() }()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return fileIdent{}, "", err
	}
	ident, err := identFromFile(f)
	if err != nil {
		return fileIdent{}, "", err
	}
	before := identFromInfo(info)
	if ident.size != before.size || ident.mtime != before.mtime {
		return fileIdent{}, "", errChanged
	}
	return ident, "sha256:" + hex.EncodeToString(sum.Sum(nil)), nil
}

func validSHA256Digest(d string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(d, prefix) {
		return false
	}
	hexPart := d[len(prefix):]
	if len(hexPart) != 64 {
		return false
	}
	for _, c := range hexPart {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func filteredEnv() []string {
	allow := map[string]struct{}{}
	for _, k := range ceEnvAllow {
		allow[k] = struct{}{}
	}
	out := make([]string, 0, len(ceEnvAllow))
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		if _, ok := allow[key]; ok {
			out = append(out, kv)
		}
	}
	return out
}

type ceProc struct {
	exit   int
	stdout []byte
	stderr []byte
}

func invokeCE(ctx context.Context, opts Options, dir string, env, args []string) (ceProc, error) {
	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, opts.CE.Path, args...) //nolint:gosec // G204: absolute trusted descriptor, fixed args
	cmd.Dir = dir
	cmd.Env = env
	setProcGroup(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &cappedWriter{buf: &stdout, cap: maxCEStreamBytes}
	cmd.Stderr = &cappedWriter{buf: &stderr, cap: maxCEStreamBytes}
	err := cmd.Run()
	result := ceProc{stdout: stdout.Bytes(), stderr: stderr.Bytes()}
	if runCtx.Err() != nil {
		killProcTree(cmd)
		return result, fmt.Errorf("ce timeout")
	}
	if cmd.ProcessState != nil {
		result.exit = cmd.ProcessState.ExitCode()
	} else if err != nil {
		return result, err
	}
	if stdoutOverflow(cmd.Stdout) || stdoutOverflow(cmd.Stderr) {
		killProcTree(cmd)
		return result, fmt.Errorf("ce stream cap exceeded")
	}
	return result, nil
}

func verifyHandshake(stdout []byte, wantCap string) error {
	var report ceVersionReport
	if err := decodeExactJSON(stdout, &report); err != nil {
		return err
	}
	for _, cap := range report.Capabilities {
		if cap == wantCap {
			return nil
		}
	}
	return fmt.Errorf("capability mismatch: want %s", wantCap)
}

func mapDoctorResult(proc ceProc) Component {
	if proc.exit == 2 {
		gz, outcome, domain := MapCEExit(proc.exit, "")
		_ = gz
		return Component{ComponentOutcome: outcome, FaultDomain: domain, Reason: "ce invocation fault"}
	}
	var report ceDoctorReport
	if err := decodeExactJSON(proc.stdout, &report); err != nil {
		return faultComponent(err.Error())
	}
	if report.CapabilityID != CECapabilityID || report.SchemaVersion != CEDoctorSchema {
		return faultComponent("ce doctor schema mismatch")
	}
	gz, outcome, domain := MapCEExit(proc.exit, report.Status)
	comp := Component{
		ComponentOutcome: outcome,
		FaultDomain:      domain,
		ProviderStatus:   report.Status,
	}
	if outcome == OutcomeFault {
		comp.Reason = "envelope/exit disagreement"
		if proc.exit != 0 && proc.exit != 1 {
			comp.Reason = "ce unexpected exit"
		}
		return comp
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return faultComponent(err.Error())
	}
	comp.Report = raw
	_ = gz
	return comp
}

func decodeExactJSON(data []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("ce json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("ce json: trailing document")
	}
	return nil
}

type cappedWriter struct {
	buf     *bytes.Buffer
	cap     int
	over    bool
	skipped int
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	remain := w.cap - w.buf.Len()
	if remain <= 0 {
		w.over = true
		w.skipped += len(p)
		return len(p), nil
	}
	if len(p) > remain {
		w.over = true
		_, _ = w.buf.Write(p[:remain])
		w.skipped += len(p) - remain
		return len(p), nil
	}
	return w.buf.Write(p)
}

func stdoutOverflow(w io.Writer) bool {
	cw, ok := w.(*cappedWriter)
	return ok && cw.over
}
