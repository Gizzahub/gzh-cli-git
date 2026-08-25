// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

// resolveBulkDepth applies defaults.scan.depth when --scan-depth was not
// explicitly supplied. The flag remains authoritative, including when it is
// invalid: an explicit zero or negative value must be reported rather than
// silently replaced by configuration.
func resolveBulkDepth(cmd *cobra.Command, directory string, depth *int) error {
	if cmd.Flags().Changed("scan-depth") {
		return validateBulkDepth(cmd, *depth)
	}

	configured, err := loadScanDepth(directory)
	if err != nil {
		return err
	}
	if configured == 0 {
		return nil
	}
	if configured < 1 {
		return fmt.Errorf("defaults.scan.depth must be at least 1, got %d", configured)
	}

	*depth = configured
	return nil
}

// loadScanDepth reads the effective defaults.scan.depth from the config
// hierarchy rooted at directory. No config file, or an omitted depth, means
// that the registered CLI default remains in effect.
func loadScanDepth(directory string) (int, error) {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return 0, fmt.Errorf("resolve scan directory: %w", err)
	}

	configDir, err := config.FindConfigRecursive(abs, ".gz-git.yaml")
	if err != nil {
		return 0, nil
	}

	cfg, err := config.LoadConfigRecursive(configDir, ".gz-git.yaml")
	if err != nil {
		return 0, fmt.Errorf("load defaults.scan.depth: %w", err)
	}
	if cfg == nil {
		return 0, nil
	}
	return cfg.GetScanDepth(), nil
}
