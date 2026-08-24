// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

// resolveScanExclude combines the --exclude flag with defaults.scan.exclude from
// the config hierarchy and returns the single regex the scanner filters on.
//
// Why the merge happens here and not in pkg/repository: pkg/config already
// imports pkg/repository, so the scanner cannot read a config file without an
// import cycle. The CLI layer is where the two meet.
//
// Why exclusions are merged rather than overridden by the flag: the point of a
// declared exclusion is to survive a forgotten flag. A user who passes
// --exclude on top of a configured one is narrowing the target set further, not
// re-opening a repository they declared off-limits. --include cannot reopen one
// either, because filterRepositories tests exclude before include.
//
// It returns a string rather than (string, error) so it drops into the option
// literal each bulk command already builds. A malformed pattern is not swallowed
// by that choice: it is reported here with the source that produced it, and left
// in the returned regex so filterRepositories refuses to scan. Failing closed
// matters more than a tidy error path — an exclusion that quietly stops applying
// is the exact failure this key exists to prevent.
func resolveScanExclude(directory, flagExclude string) string {
	patterns := loadScanExcludePatterns(directory)
	if len(patterns) == 0 {
		return flagExclude
	}

	// Report before validating: an operator who declared an exclusion needs to
	// see it took effect on every run, not only on the runs where it parses.
	reportScanExclude(patterns)

	for _, pattern := range patterns {
		if _, err := regexp.Compile(pattern); err != nil {
			fmt.Fprintf(os.Stderr,
				"error: defaults.scan.exclude pattern %q is not a valid regex: %v\n", pattern, err)
		}
	}

	combined := patterns
	if flagExclude != "" {
		combined = append(append([]string{}, patterns...), flagExclude)
	}
	return combineExcludePatterns(combined)
}

// combineExcludePatterns joins patterns into one alternation.
//
// Each branch is wrapped in a non-capturing group so an unanchored alternation
// inside a single pattern cannot swallow the ones after it: without the group,
// `a|b` followed by `c` would compile as `a|bc`, silently dropping the `c`
// exclusion and letting a repository through.
func combineExcludePatterns(patterns []string) string {
	if len(patterns) == 1 {
		return patterns[0]
	}
	grouped := make([]string, 0, len(patterns))
	for _, p := range patterns {
		grouped = append(grouped, "(?:"+p+")")
	}
	return strings.Join(grouped, "|")
}

// loadScanExcludePatterns reads defaults.scan.exclude from the config hierarchy
// rooted at directory.
//
// A missing or unreadable config is not an error: every bulk command runs
// happily without a config file, and failing the whole command because an
// optional key could not be read would be a worse outcome than the exclusion
// this function is trying to supply. An unreadable config that *does* exist is
// reported, because that one is a mistake the user wants to hear about.
func loadScanExcludePatterns(directory string) []string {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return nil
	}

	configDir, err := config.FindConfigRecursive(abs, ".gz-git.yaml")
	if err != nil {
		return nil
	}

	cfg, err := config.LoadConfigRecursive(configDir, ".gz-git.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: ignoring defaults.scan.exclude, config failed to load: %v\n", err)
		return nil
	}
	if cfg == nil {
		return nil
	}
	return cfg.GetScanExcludePatterns()
}

// reportScanExclude announces the active exclusions on stderr.
//
// stderr rather than stdout so it cannot corrupt --format json/llm output,
// which is parsed by other tools. It is printed on every run, not only
// --dry-run: a repository quietly missing from a push is exactly as surprising
// as one quietly missing from a preview.
func reportScanExclude(patterns []string) {
	if quiet {
		return
	}
	fmt.Fprintf(os.Stderr, "Excluding repositories matching defaults.scan.exclude: %s\n",
		strings.Join(patterns, ", "))
}
