// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"

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
		defaultDepth, err := registeredScanDepthDefault(cmd, *depth)
		if err != nil {
			return err
		}
		*depth = defaultDepth
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
	configPath, err := config.DetectConfigFile(directory)
	if err != nil {
		// Bulk scans do not require a project config. DetectConfigFile uses an
		// error to report the ordinary "not found" case, which means keep the
		// registered CLI default here.
		return 0, nil //nolint:nilerr // an absent optional config is not a command failure
	}

	cfg, err := config.LoadConfigRecursive(filepath.Dir(configPath), filepath.Base(configPath))
	if err != nil {
		return 0, fmt.Errorf("load defaults.scan.depth: %w", err)
	}
	if cfg == nil {
		return 0, nil
	}
	return cfg.GetScanDepth(), nil
}

func registeredScanDepthDefault(cmd *cobra.Command, fallback int) (int, error) {
	flag := cmd.Flags().Lookup("scan-depth")
	if flag == nil {
		return fallback, nil
	}
	depth, err := strconv.Atoi(flag.DefValue)
	if err != nil {
		return 0, fmt.Errorf("parse scan-depth default %q: %w", flag.DefValue, err)
	}
	return depth, nil
}
