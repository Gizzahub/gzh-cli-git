// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

// Package gzhcligitforge provides git forge operations and synchronization utilities.
package gzhcligitforge

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Version information.
// These values can be overridden at build time using -ldflags.
//
// Example:
//
//	go build -ldflags "-X github.com/gizzahub/gzh-cli-gitforge.GitCommit=$(git rev-parse HEAD)"
var (
	// Version is the current library version following semantic versioning.
	// Format: vMAJOR.MINOR.PATCH[-PRERELEASE].
	Version = defaultVersion

	// GitCommit is the git commit SHA of the build.
	// This is set during the build process.
	GitCommit = defaultUnknown

	// BuildDate is the date when the binary was built.
	// This is set during the build process.
	BuildDate = defaultUnknown
)

// Defaults the linker overwrites. They are named so the build-info fallback can
// tell "nobody set this" apart from "the build deliberately said this".
const (
	defaultVersion = "0.7.0"
	defaultUnknown = "unknown"
)

// One advertised install path cannot pass -ldflags at all:
//
//	go install github.com/gizzahub/gzh-cli-gitforge/cmd/gz-git@<tag>
//
// builds from the module proxy, so a binary installed that way used to report
// the constants above forever no matter which tag produced it. The toolchain
// already stamps the same facts into the binary -- the module version for a
// versioned install, and the VCS revision and commit time for a local build --
// so read those back instead of shipping a binary that misreports itself.
//
// The VCS half of that is best-effort. The toolchain stamps vcs.* only when it
// recognizes the package directory as a repository, and it does not recognize a
// linked git worktree, where .git is a file rather than a directory -- a plain
// `go build` there produces no vcs settings and no error. Development builds are
// expected to come from `make build`, which injects the values outright; the
// module version above is what the install path this fallback exists for relies
// on, and that needs no VCS stamp at all.
func init() {
	if bi, ok := debug.ReadBuildInfo(); ok {
		Version, GitCommit, BuildDate = versionFromBuildInfo(bi, Version, GitCommit, BuildDate)
	}
}

// versionFromBuildInfo fills in only the fields still holding their defaults, so
// -ldflags always wins over the embedded stamp. It is separated from init for
// testing; it must not mutate package state.
func versionFromBuildInfo(bi *debug.BuildInfo, version, commit, date string) (outVersion, outCommit, outDate string) {
	// "(devel)" is what the toolchain records for a build that is not from a
	// tagged module version -- it carries no more information than the default.
	if version == defaultVersion && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		version = strings.TrimPrefix(bi.Main.Version, "v")
	}

	var revision, modified string

	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.time":
			if date == defaultUnknown && setting.Value != "" {
				date = setting.Value
			}
		case "vcs.modified":
			modified = setting.Value
		}
	}

	if commit == defaultUnknown && revision != "" {
		commit = shortRevision(revision)
		// The recorded revision describes the commit, not the tree that was
		// actually compiled. Saying so is the point of reporting a commit.
		if modified == "true" {
			commit += "-dirty"
		}
	}

	return version, commit, date
}

// shortRevision trims a full SHA to the 7-character form the rest of this
// package documents and the Makefile injects.
func shortRevision(revision string) string {
	const shortLen = 7
	if len(revision) <= shortLen {
		return revision
	}

	return revision[:shortLen]
}

// VersionInfo returns detailed version information as a map.
//
// The returned map contains:
//   - version: The library version (e.g., "0.1.0-alpha")
//   - gitCommit: The git commit SHA (e.g., "a1b2c3d")
//   - buildDate: The build date (e.g., "2025-11-30")
//   - goVersion: The Go version used for building (e.g., "go1.24.0")
//
// Example:
//
//	info := gzhcligitforge.VersionInfo()
//	fmt.Printf("Version: %s\n", info["version"])
//	fmt.Printf("Commit: %s\n", info["gitCommit"])
func VersionInfo() map[string]string {
	return map[string]string{
		"version":   Version,
		"gitCommit": GitCommit,
		"buildDate": BuildDate,
		"goVersion": runtime.Version(),
	}
}

// VersionString returns a formatted version string.
//
// Format: "gzh-cli-gitforge version v0.1.0-alpha (commit: a1b2c3d, built: 2025-11-30)"
//
// Example:
//
//	fmt.Println(gzhcligitforge.VersionString())
//	// Output: gzh-cli-gitforge version v0.1.0-alpha (commit: unknown, built: unknown)
func VersionString() string {
	return fmt.Sprintf("gzh-cli-gitforge version v%s (commit: %s, built: %s)",
		Version, GitCommit, BuildDate)
}

// ShortVersion returns just the version number without prefix.
//
// Example:
//
//	fmt.Println(gzhcligitforge.ShortVersion())
//	// Output: 0.1.0-alpha
func ShortVersion() string {
	return Version
}

// FullVersion returns the version with 'v' prefix.
//
// Example:
//
//	fmt.Println(gzhcligitforge.FullVersion())
//	// Output: v0.1.0-alpha
func FullVersion() string {
	return "v" + Version
}
