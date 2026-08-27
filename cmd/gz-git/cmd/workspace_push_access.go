// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

func configuredWorkspacePushAccess(repoPath string) (allowed bool, reason string, err error) {
	access, configPath, err := config.WorkspacePushAccess(repoPath)
	if err != nil {
		return false, "", err
	}
	if !access.IsReadOnly() {
		return true, "", nil
	}
	return false, fmt.Sprintf("read-only workspace declared in %s; push disabled", configPath), nil
}
