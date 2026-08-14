// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package handoff

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/safefs"
)

const (
	// largeFileThreshold is the size above which a file is assumed to be a build
	// output or a binary blob rather than something meant for version control.
	largeFileThreshold = 5 << 20 // 5 MiB

	// contentScanLimit caps how much of a file is read looking for credentials.
	// Keys and tokens live near the top of config files, and reading further
	// would make the guard slow enough that people skip it.
	contentScanLimit = 1 << 20 // 1 MiB

	// binarySniffLimit is how much of a file is examined for NUL bytes before
	// deciding it is binary and not worth a content scan.
	binarySniffLimit = 8000
)

// Guard reports everything in repoPath that an automatic commit would sweep up
// but a person would not have staged deliberately.
//
// This is what separates an explicit checkpoint command from a background
// auto-commit loop: the sweep still happens, but never silently.
func Guard(ctx context.Context, exec *gitcmd.Executor, repoPath string) ([]Finding, error) {
	pending, err := pendingFiles(ctx, exec, repoPath)
	if err != nil {
		return nil, err
	}

	root, err := safefs.OpenRoot(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open repository root: %w", err)
	}
	defer func() { _ = root.Close() }()

	return inspectFilesAt(root, pending), nil
}

// pendingFile is one path that "git add -A" would stage.
type pendingFile struct {
	path string
	// untracked distinguishes a file git has never seen from a modification to
	// one it already tracks. Only the former can be an accidental artifact.
	untracked bool
}

func pendingFiles(ctx context.Context, exec *gitcmd.Executor, repoPath string) ([]pendingFile, error) {
	// -uall lists every file inside an untracked directory instead of the
	// directory alone, which is the difference between seeing "node_modules/"
	// and seeing what is actually about to be committed.
	result, err := exec.Run(ctx, repoPath, "status", "--porcelain=v1", "-uall", "-z")
	if err != nil {
		return nil, fmt.Errorf("failed to list pending files: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("failed to list pending files: %s", strings.TrimSpace(result.Stderr))
	}
	return parsePorcelainZ(result.Stdout), nil
}

// parsePorcelainZ splits NUL-terminated status records into candidate paths.
//
// Rename and copy records carry a second path field. Which of the two comes
// first has varied across git versions, so both are kept and the ones that no
// longer exist on disk are dropped during inspection.
func parsePorcelainZ(out string) []pendingFile {
	var files []pendingFile

	for record := range strings.SplitSeq(out, "\x00") {
		if len(record) < 4 {
			// Either padding after the final NUL or a bare rename source path,
			// which is picked up as its own record and inspected the same way.
			if record != "" {
				files = append(files, pendingFile{path: record})
			}
			continue
		}

		x, y := record[0], record[1]
		if x == 'D' || y == 'D' {
			// A deletion has nothing left to inspect.
			continue
		}

		files = append(files, pendingFile{
			path:      record[3:],
			untracked: x == '?' && y == '?',
		})
	}

	return files
}

// inspectFiles classifies pending paths. It takes the repository root and the
// file list rather than running git itself, so the classification rules can be
// tested against files on disk without a repository.
func inspectFiles(repoPath string, pending []pendingFile) []Finding {
	root, err := safefs.OpenRoot(repoPath)
	if err != nil {
		return nil
	}
	defer func() { _ = root.Close() }()

	return inspectFilesAt(root, pending)
}

func inspectFilesAt(root *safefs.Root, pending []pendingFile) []Finding {
	var findings []Finding

	for _, file := range pending {
		info, err := root.Stat(file.path)
		if err != nil || info.IsDir() {
			// Gone, unreadable, or a directory record: nothing to weigh.
			continue
		}

		if detail := classifyName(file.path); detail != "" {
			findings = append(findings, Finding{Kind: FindingSecret, File: file.path, Detail: detail})
			continue
		}

		if info.Size() > largeFileThreshold {
			findings = append(findings, Finding{
				Kind:   FindingLargeFile,
				File:   file.path,
				Detail: fmt.Sprintf("%s is larger than the %s commit threshold", humanSize(info.Size()), humanSize(largeFileThreshold)),
			})
			continue
		}

		if file.untracked {
			if detail := classifyArtifact(file.path); detail != "" {
				findings = append(findings, Finding{Kind: FindingArtifact, File: file.path, Detail: detail})
				continue
			}
		}

		if detail := scanContentAt(root, file.path, info.Size()); detail != "" {
			findings = append(findings, Finding{Kind: FindingSecret, File: file.path, Detail: detail})
		}
	}

	return findings
}

// secretNames matches files whose name alone says they hold a credential.
var secretNames = []struct {
	glob   string
	detail string
}{
	{".env", "environment files usually hold credentials"},
	{".env.*", "environment files usually hold credentials"},
	{".netrc", "holds machine login credentials"},
	{".npmrc", "may hold a registry auth token"},
	{".pypirc", "may hold a package index password"},
	{"id_rsa", "private SSH key"},
	{"id_dsa", "private SSH key"},
	{"id_ecdsa", "private SSH key"},
	{"id_ed25519", "private SSH key"},
	{"credentials", "credential store"},
	{"credentials.json", "credential store"},
	{"*.pem", "may hold a private key or certificate"},
	{"*.key", "may hold a private key"},
	{"*.p12", "key store"},
	{"*.pfx", "key store"},
	{"*.jks", "Java key store"},
	{"*.keystore", "key store"},
	{"*service-account*.json", "service account key"},
}

// secretNameExceptions are the placeholder files projects commit on purpose.
var secretNameExceptions = []string{
	".env.example", ".env.sample", ".env.template", ".env.dist",
}

// classifyName reports why a path looks like a credential file, or "" if it
// does not.
func classifyName(path string) string {
	base := filepath.Base(path)

	for _, allowed := range secretNameExceptions {
		if strings.EqualFold(base, allowed) {
			return ""
		}
	}

	for _, candidate := range secretNames {
		if ok, err := filepath.Match(candidate.glob, base); err == nil && ok {
			return candidate.detail
		}
	}

	return ""
}

// artifactDirs are directory names that hold generated output. A file that
// appears under one of them and is not ignored means the .gitignore is missing
// an entry, not that the file was meant to be committed.
var artifactDirs = map[string]bool{
	"node_modules": true, "bower_components": true,
	"dist": true, "build": true, "target": true, "out": true,
	"__pycache__": true, ".pytest_cache": true, ".mypy_cache": true, ".tox": true,
	".venv": true, "venv": true, ".next": true, ".nuxt": true,
	".gradle": true, ".terraform": true, "coverage": true,
}

// artifactExts are file extensions that are always compiler or runtime output.
var artifactExts = map[string]bool{
	".pyc": true, ".class": true, ".o": true, ".obj": true,
	".so": true, ".dylib": true, ".dll": true, ".exe": true,
	".log": true, ".tmp": true, ".swp": true,
}

// classifyArtifact reports why an untracked path looks generated, or "".
func classifyArtifact(path string) string {
	for segment := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if artifactDirs[segment] {
			return fmt.Sprintf("generated output under %s/, which is not in .gitignore", segment)
		}
	}

	if ext := strings.ToLower(filepath.Ext(path)); artifactExts[ext] {
		return fmt.Sprintf("%s files are build output, and this one is not in .gitignore", ext)
	}

	if filepath.Base(path) == ".DS_Store" {
		return "Finder metadata, which is not in .gitignore"
	}

	return ""
}

// contentSecrets are patterns specific enough that a match is worth stopping
// for. Generic "password =" style rules are deliberately absent: they fire on
// test fixtures often enough that people learn to bypass the guard.
var contentSecrets = []struct {
	re     *regexp.Regexp
	detail string
}{
	{regexp.MustCompile(`-{5}BEGIN [A-Z ]*PRIVATE KEY-{5}`), "contains a private key block"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "contains an AWS access key ID"},
	{regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36}`), "contains a GitHub token"},
	{regexp.MustCompile(`glpat-[A-Za-z0-9_\-]{20}`), "contains a GitLab access token"},
	{regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`), "contains a Slack token"},
	{regexp.MustCompile(`AIza[0-9A-Za-z_\-]{35}`), "contains a Google API key"},
}

// scanContent reports why a file's contents look like a credential, or "".
func scanContent(full string, size int64) string {
	if size == 0 {
		return ""
	}

	root, err := safefs.OpenRoot(filepath.Dir(full))
	if err != nil {
		return ""
	}
	defer func() { _ = root.Close() }()

	return scanContentAt(root, filepath.Base(full), size)
}

func scanContentAt(root *safefs.Root, path string, size int64) string {
	file, err := root.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	buf := make([]byte, min(size, contentScanLimit))
	n, err := file.Read(buf)
	if n == 0 || (err != nil && n == 0) {
		return ""
	}
	buf = buf[:n]

	if isBinary(buf) {
		return ""
	}

	for _, candidate := range contentSecrets {
		if candidate.re.Match(buf) {
			return candidate.detail
		}
	}

	return ""
}

// isBinary reports whether the head of a file contains a NUL byte, the usual
// heuristic git itself uses to decide a file is not text.
func isBinary(buf []byte) bool {
	head := buf
	if len(head) > binarySniffLimit {
		head = head[:binarySniffLimit]
	}
	return bytes.IndexByte(head, 0) >= 0
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGT"[exp])
}
