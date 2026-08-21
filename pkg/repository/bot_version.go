// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import (
	"strings"
	"unicode"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

const (
	dependabotGoModulesPrefix = "dependabot/go_modules/"
	dependabotActionsPrefix   = "dependabot/github_actions/"
)

type botVersionFiles struct {
	baseGoMod     string
	botGoMod      string
	baseWorkflows []string
}

type actionPin struct {
	action  string
	version string
}

type parsedActionVer struct {
	canonical string
	major     string
	majorOnly bool
}

// botTargetSuperseded reports whether base already satisfies the version this
// bot branch was opened to deliver. Comparison is versions, not git ancestry.
// Unknown ecosystems and incomparable pins (including Actions major-tag jumps)
// return false so the ref stays pending.
func botTargetSuperseded(name string, files botVersionFiles) bool {
	n := botMatchName(name)
	switch {
	case strings.HasPrefix(n, dependabotGoModulesPrefix):
		return goModuleTargetSuperseded(n, files.baseGoMod, files.botGoMod)
	case strings.HasPrefix(n, dependabotActionsPrefix):
		return actionsTargetSuperseded(n, files.baseWorkflows)
	default:
		return false
	}
}

func botKindComparable(name string) bool {
	n := botMatchName(name)
	return strings.HasPrefix(n, dependabotGoModulesPrefix) ||
		strings.HasPrefix(n, dependabotActionsPrefix)
}

func goModuleTargetSuperseded(branch, baseMod, botMod string) bool {
	baseVer, botVer, ok := resolveGoModuleTarget(branch, baseMod, botMod)
	if !ok {
		return false
	}
	return goVersionGTE(baseVer, botVer)
}

func resolveGoModuleTarget(branch, baseMod, botMod string) (baseVer, botVer string, ok bool) {
	baseReq := parseGoModRequires(baseMod)
	botReq := parseGoModRequires(botMod)
	path, nameVer, parsed := parseGoModuleBotTarget(branch)
	if parsed {
		return resolveNamedGoModule(path, nameVer, baseReq, botReq)
	}
	botVer, baseVer = uniqueGoModBump(baseReq, botReq)
	return baseVer, botVer, botVer != "" && baseVer != ""
}

func resolveNamedGoModule(path, nameVer string, baseReq, botReq map[string]string) (baseVer, botVer string, ok bool) {
	botVer = nameVer
	if m, v, hit := lookupModule(botReq, path); hit {
		path = m
		botVer = v
	}
	if v, hit := baseReq[path]; hit {
		return v, botVer, botVer != ""
	}
	if _, v, hit := lookupModule(baseReq, path); hit {
		return v, botVer, botVer != ""
	}
	return "", botVer, false
}

func uniqueGoModBump(baseReq, botReq map[string]string) (botVer, baseVer string) {
	if len(baseReq) == 0 || len(botReq) == 0 {
		return "", ""
	}
	found := ""
	for m, bv := range botReq {
		av, hit := baseReq[m]
		if !hit || av == bv {
			continue
		}
		if found != "" {
			return "", ""
		}
		found = m
		botVer = bv
		baseVer = av
	}
	return botVer, baseVer
}

func lookupModule(requires map[string]string, candidate string) (module, version string, ok bool) {
	if candidate == "" || len(requires) == 0 {
		return "", "", false
	}
	if v, hit := requires[candidate]; hit {
		return candidate, v, true
	}
	rest := candidate
	for {
		i := strings.IndexByte(rest, '/')
		if i < 0 {
			return "", "", false
		}
		rest = rest[i+1:]
		if !strings.Contains(rest, ".") && !strings.Contains(rest, "/") {
			continue
		}
		if v, hit := requires[rest]; hit {
			return rest, v, true
		}
	}
}

func parseGoModuleBotTarget(branch string) (modulePath, version string, ok bool) {
	name := botMatchName(branch)
	rest, found := strings.CutPrefix(name, dependabotGoModulesPrefix)
	if !found || rest == "" {
		return "", "", false
	}
	for {
		i := strings.LastIndexByte(rest, '-')
		if i <= 0 {
			return "", "", false
		}
		cand := rest[i+1:]
		if looksLikeGoModuleVersion(cand) {
			path := rest[:i]
			if path == "" {
				return "", "", false
			}
			return path, canonicalizeGoVersion(cand), true
		}
		rest = rest[:i]
	}
}

func looksLikeGoModuleVersion(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	token := s
	if len(token) > 1 && (token[0] == 'v' || token[0] == 'V') && token[1] >= '0' && token[1] <= '9' {
		token = token[1:]
	}
	// Bare majors (v2) appear in module paths such as aws-sdk-go-v2.
	// Check the raw token: semver.Canonical("v2") is "v2.0.0" and would
	// otherwise look like a real version.
	if !strings.Contains(token, ".") {
		return false
	}
	return canonicalizeGoVersion(s) != ""
}

func canonicalizeGoVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !semver.IsValid(v) && !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return ""
	}
	return semver.Canonical(v)
}

func goVersionGTE(base, bot string) bool {
	bv := canonicalizeGoVersion(base)
	tv := canonicalizeGoVersion(bot)
	if bv == "" || tv == "" {
		return false
	}
	return semver.Compare(bv, tv) >= 0
}

func parseGoModRequires(content string) map[string]string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	f, err := modfile.Parse("go.mod", []byte(content), nil)
	if err != nil || f == nil {
		return nil
	}
	out := make(map[string]string, len(f.Require))
	for _, r := range f.Require {
		if r == nil || r.Mod.Path == "" || r.Mod.Version == "" {
			continue
		}
		out[r.Mod.Path] = r.Mod.Version
	}
	return out
}

func actionsTargetSuperseded(branch string, baseWorkflows []string) bool {
	action, botVer, ok := parseGitHubActionsBotTarget(branch)
	if !ok {
		return false
	}
	sawPin := false
	for _, body := range baseWorkflows {
		for _, pin := range parseWorkflowUses(body) {
			if pin.action != action {
				continue
			}
			sawPin = true
			rel, comparable := compareActionVersion(pin.version, botVer)
			if !comparable || rel < 0 {
				return false
			}
		}
	}
	return sawPin
}

func parseGitHubActionsBotTarget(branch string) (action, version string, ok bool) {
	name := botMatchName(branch)
	rest, found := strings.CutPrefix(name, dependabotActionsPrefix)
	if !found || rest == "" {
		return "", "", false
	}
	for {
		i := strings.LastIndexByte(rest, '-')
		if i <= 0 {
			return "", "", false
		}
		cand := rest[i+1:]
		if looksLikeActionVersion(cand) {
			action = rest[:i]
			if action == "" || !strings.Contains(action, "/") {
				return "", "", false
			}
			return action, cand, true
		}
		rest = rest[:i]
	}
}

func looksLikeActionVersion(s string) bool {
	if _, ok := parseActionVersion(s); ok {
		return true
	}
	return false
}

func parseWorkflowUses(content string) []actionPin {
	var pins []actionPin
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if i := strings.IndexByte(trimmed, '#'); i >= 0 {
			trimmed = strings.TrimSpace(trimmed[:i])
		}
		key, rest, found := strings.Cut(trimmed, "uses:")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key != "" && key != "-" {
			continue
		}
		rest = strings.Trim(strings.TrimSpace(rest), `"'`)
		if rest == "" || strings.HasPrefix(rest, "docker://") || strings.HasPrefix(rest, "./") {
			continue
		}
		action, version, found := strings.Cut(rest, "@")
		if !found || action == "" || version == "" {
			continue
		}
		pins = append(pins, actionPin{
			action:  strings.TrimSuffix(action, "/"),
			version: version,
		})
	}
	return pins
}

// compareActionVersion returns semver.Compare(base, bot) when the pins are
// comparable. Major-only tags with different majors (v4 vs v7) are not.
func compareActionVersion(base, bot string) (int, bool) {
	b, bok := parseActionVersion(base)
	t, tok := parseActionVersion(bot)
	if !bok || !tok {
		return 0, false
	}
	if (b.majorOnly || t.majorOnly) && b.major != t.major {
		return 0, false
	}
	return semver.Compare(b.canonical, t.canonical), true
}

func parseActionVersion(raw string) (parsedActionVer, bool) {
	raw = strings.Trim(strings.TrimSpace(raw), `"'`)
	if raw == "" || isGitSHA(raw) || raw == "latest" || raw == "main" || raw == "master" {
		return parsedActionVer{}, false
	}
	v := raw
	if !semver.IsValid(v) && actionVersionNeedsV(v) {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return parsedActionVer{}, false
	}
	return parsedActionVer{
		canonical: semver.Canonical(v),
		major:     semver.Major(v),
		majorOnly: actionMajorOnly(raw),
	}, true
}

func actionVersionNeedsV(v string) bool {
	if v == "" {
		return false
	}
	r := v[0]
	return r >= '0' && r <= '9'
}

func actionMajorOnly(raw string) bool {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isGitSHA(s string) bool {
	n := len(s)
	if n < 7 || n > 40 {
		return false
	}
	for _, c := range s {
		if !unicode.Is(unicode.ASCII_Hex_Digit, c) {
			return false
		}
	}
	return true
}
