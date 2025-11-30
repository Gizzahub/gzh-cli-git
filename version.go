// Package gzhcligit provides version information for gzh-cli-git library and CLI tool.
package gzhcligit

import (
	"fmt"
	"runtime"
)

// Version information.
// These values can be overridden at build time using -ldflags.
//
// Example:
//
//	go build -ldflags "-X github.com/gizzahub/gzh-cli-git.GitCommit=$(git rev-parse HEAD)"
var (
	// Version is the current library version following semantic versioning.
	// Format: vMAJOR.MINOR.PATCH[-PRERELEASE]
	Version = "0.1.0-alpha"

	// GitCommit is the git commit SHA of the build.
	// This is set during the build process.
	GitCommit = "unknown"

	// BuildDate is the date when the binary was built.
	// This is set during the build process.
	BuildDate = "unknown"
)

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
//	info := gzhcligit.VersionInfo()
//	fmt.Printf("Version: %s\n", info["version"])
//	fmt.Printf("Commit: %s\n", info["gitCommit"])
func VersionInfo() map[string]string {
	return map[string]string{
		"version":    Version,
		"gitCommit":  GitCommit,
		"buildDate":  BuildDate,
		"goVersion":  runtime.Version(),
	}
}

// VersionString returns a formatted version string.
//
// Format: "gzh-cli-git version v0.1.0-alpha (commit: a1b2c3d, built: 2025-11-30)"
//
// Example:
//
//	fmt.Println(gzhcligit.VersionString())
//	// Output: gzh-cli-git version v0.1.0-alpha (commit: unknown, built: unknown)
func VersionString() string {
	return fmt.Sprintf("gzh-cli-git version v%s (commit: %s, built: %s)",
		Version, GitCommit, BuildDate)
}

// ShortVersion returns just the version number without prefix.
//
// Example:
//
//	fmt.Println(gzhcligit.ShortVersion())
//	// Output: 0.1.0-alpha
func ShortVersion() string {
	return Version
}

// FullVersion returns the version with 'v' prefix.
//
// Example:
//
//	fmt.Println(gzhcligit.FullVersion())
//	// Output: v0.1.0-alpha
func FullVersion() string {
	return "v" + Version
}
