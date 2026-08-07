// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

// Run executes all diagnostic checks and returns a report.
func Run(ctx context.Context, opts Options) *Report {
	start := time.Now()

	var checks []CheckResult

	// System checks
	checks = append(checks, checkGitInstalled(ctx)...)
	checks = append(checks, checkGitWorkflowSettings(ctx, opts.Verbose)...)
	checks = append(checks, checkSSH()...)
	checks = append(checks, checkTempDir()...)
	checks = append(checks, checkConfigDir()...)

	// Config checks
	checks = append(checks, checkGlobalConfig()...)
	checks = append(checks, checkActiveProfile()...)
	checks = append(checks, checkProfiles(opts.Verbose)...)
	checks = append(checks, checkProjectConfig(opts.Directory)...)

	// Auth checks (from profile)
	checks = append(checks, checkSSHKeys()...)
	checks = append(checks, checkTokenSources()...)

	// Forge checks
	if !opts.SkipForge {
		checks = append(checks, checkForgeConnectivity(ctx)...)
	}

	// Repository checks
	if !opts.SkipRepo {
		checks = append(checks, checkRepositories(ctx, opts)...)
	}

	report := &Report{
		Checks:   checks,
		Duration: time.Since(start),
	}
	report.Summary = summarize(checks)

	return report
}

func summarize(checks []CheckResult) Summary {
	s := Summary{Total: len(checks)}
	for _, c := range checks {
		switch c.Status {
		case StatusOK:
			s.OK++
		case StatusWarning:
			s.Warning++
		case StatusError:
			s.Error++
		case StatusUnreachable:
			s.Unreachable++
		case StatusSkipped:
			s.Skipped++
		}
	}
	return s
}

// --- System Checks ---

func checkGitInstalled(ctx context.Context) []CheckResult {
	executor := gitcmd.NewExecutor()
	version, err := executor.GetGitVersion(ctx)
	if err != nil {
		return []CheckResult{{
			Name:     "git",
			Category: CategorySystem,
			Status:   StatusError,
			Message:  "git is not installed or not in PATH",
			Detail:   err.Error(),
		}}
	}
	return []CheckResult{{
		Name:     "git",
		Category: CategorySystem,
		Status:   StatusOK,
		Message:  fmt.Sprintf("git version %s", version),
	}}
}

func checkSSH() []CheckResult {
	_, err := exec.LookPath("ssh")
	if err != nil {
		return []CheckResult{{
			Name:     "ssh",
			Category: CategorySystem,
			Status:   StatusWarning,
			Message:  "ssh not found in PATH (required for SSH clone)",
		}}
	}
	return []CheckResult{{
		Name:     "ssh",
		Category: CategorySystem,
		Status:   StatusOK,
		Message:  "ssh available",
	}}
}

func checkTempDir() []CheckResult {
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, ".gz-git-doctor-test")
	if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
		return []CheckResult{{
			Name:     "temp-dir",
			Category: CategorySystem,
			Status:   StatusError,
			Message:  fmt.Sprintf("cannot write to temp directory: %s", tmpDir),
			Detail:   err.Error(),
		}}
	}
	os.Remove(testFile)

	return []CheckResult{{
		Name:     "temp-dir",
		Category: CategorySystem,
		Status:   StatusOK,
		Message:  fmt.Sprintf("temp directory writable (%s)", tmpDir),
	}}
}

func checkConfigDir() []CheckResult {
	paths, err := config.NewPaths()
	if err != nil {
		return []CheckResult{{
			Name:     "config-dir",
			Category: CategorySystem,
			Status:   StatusError,
			Message:  "cannot determine config directory",
			Detail:   err.Error(),
		}}
	}

	if !paths.Exists() {
		return []CheckResult{{
			Name:     "config-dir",
			Category: CategorySystem,
			Status:   StatusWarning,
			Message:  fmt.Sprintf("config directory does not exist: %s", paths.ConfigDir),
			Detail:   "Run 'gz-git config init' to create it",
		}}
	}

	// Check permissions (unix only)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(paths.ConfigDir)
		if err == nil {
			perm := info.Mode().Perm()
			if perm&0o077 != 0 {
				return []CheckResult{{
					Name:     "config-dir",
					Category: CategorySystem,
					Status:   StatusWarning,
					Message:  fmt.Sprintf("config directory has loose permissions: %04o (recommend 0700)", perm),
					Detail:   paths.ConfigDir,
				}}
			}
		}
	}

	return []CheckResult{{
		Name:     "config-dir",
		Category: CategorySystem,
		Status:   StatusOK,
		Message:  fmt.Sprintf("config directory: %s", paths.ConfigDir),
	}}
}

// --- Config Checks ---

func checkGlobalConfig() []CheckResult {
	paths, err := config.NewPaths()
	if err != nil {
		return []CheckResult{{
			Name:     "global-config",
			Category: CategoryConfig,
			Status:   StatusError,
			Message:  "cannot determine config paths",
		}}
	}

	if paths.GlobalConfigFile == "" {
		return []CheckResult{{
			Name:     "global-config",
			Category: CategoryConfig,
			Status:   StatusWarning,
			Message:  "no global config file found",
			Detail:   "Run 'gz-git config init' to create one",
		}}
	}

	// Try loading it
	manager, err := config.NewManager()
	if err != nil {
		return []CheckResult{{
			Name:     "global-config",
			Category: CategoryConfig,
			Status:   StatusError,
			Message:  "failed to create config manager",
			Detail:   err.Error(),
		}}
	}

	_, err = manager.LoadGlobalConfig()
	if err != nil {
		return []CheckResult{{
			Name:     "global-config",
			Category: CategoryConfig,
			Status:   StatusError,
			Message:  "global config file is invalid",
			Detail:   err.Error(),
		}}
	}

	return []CheckResult{{
		Name:     "global-config",
		Category: CategoryConfig,
		Status:   StatusOK,
		Message:  fmt.Sprintf("global config: %s", paths.GlobalConfigFile),
	}}
}

func checkActiveProfile() []CheckResult {
	manager, err := config.NewManager()
	if err != nil {
		return nil
	}

	profileName, err := manager.GetActiveProfile()
	if err != nil {
		return []CheckResult{{
			Name:     "active-profile",
			Category: CategoryConfig,
			Status:   StatusError,
			Message:  "cannot read active profile",
			Detail:   err.Error(),
		}}
	}

	if profileName == "" {
		return []CheckResult{{
			Name:     "active-profile",
			Category: CategoryConfig,
			Status:   StatusWarning,
			Message:  "no active profile set",
		}}
	}

	if !manager.ProfileExists(profileName) {
		return []CheckResult{{
			Name:     "active-profile",
			Category: CategoryConfig,
			Status:   StatusError,
			Message:  fmt.Sprintf("active profile '%s' does not exist", profileName),
			Detail:   "The active profile file is missing. Create it or switch profiles.",
		}}
	}

	return []CheckResult{{
		Name:     "active-profile",
		Category: CategoryConfig,
		Status:   StatusOK,
		Message:  fmt.Sprintf("active profile: %s", profileName),
	}}
}

func checkProfiles(verbose bool) []CheckResult {
	manager, err := config.NewManager()
	if err != nil {
		return nil
	}

	profiles, err := manager.ListProfiles()
	if err != nil {
		return []CheckResult{{
			Name:     "profiles",
			Category: CategoryConfig,
			Status:   StatusWarning,
			Message:  "cannot list profiles",
			Detail:   err.Error(),
		}}
	}

	if len(profiles) == 0 {
		return []CheckResult{{
			Name:     "profiles",
			Category: CategoryConfig,
			Status:   StatusWarning,
			Message:  "no profiles found",
		}}
	}

	var results []CheckResult

	// Validate each profile
	validator := config.NewValidator()
	for _, name := range profiles {
		profile, err := manager.LoadProfile(name)
		if err != nil {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("profile:%s", name),
				Category: CategoryConfig,
				Status:   StatusError,
				Message:  fmt.Sprintf("profile '%s' failed to load", name),
				Detail:   err.Error(),
			})
			continue
		}

		if err := validator.ValidateProfile(profile); err != nil {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("profile:%s", name),
				Category: CategoryConfig,
				Status:   StatusError,
				Message:  fmt.Sprintf("profile '%s' is invalid", name),
				Detail:   err.Error(),
			})
			continue
		}

		// Check for empty token env var references
		if profile.Token != "" && strings.HasPrefix(profile.Token, "${") {
			envVar := strings.TrimSuffix(strings.TrimPrefix(profile.Token, "${"), "}")
			if os.Getenv(envVar) == "" {
				results = append(results, CheckResult{
					Name:     fmt.Sprintf("profile:%s", name),
					Category: CategoryConfig,
					Status:   StatusWarning,
					Message:  fmt.Sprintf("profile '%s': env var $%s is empty", name, envVar),
				})
				continue
			}
		}

		if verbose {
			msg := fmt.Sprintf("profile '%s': valid", name)
			if profile.Provider != "" {
				msg += fmt.Sprintf(" (provider: %s)", profile.Provider)
			}
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("profile:%s", name),
				Category: CategoryConfig,
				Status:   StatusOK,
				Message:  msg,
			})
		}
	}

	// Summary if not verbose
	if !verbose && len(results) == 0 {
		results = append(results, CheckResult{
			Name:     "profiles",
			Category: CategoryConfig,
			Status:   StatusOK,
			Message:  fmt.Sprintf("%d profile(s) found, all valid", len(profiles)),
		})
	}

	return results
}

func checkProjectConfig(_ string) []CheckResult {
	projectConfigPath, err := config.FindProjectConfig()
	if err != nil || projectConfigPath == "" {
		return []CheckResult{{
			Name:     "project-config",
			Category: CategoryConfig,
			Status:   StatusOK,
			Message:  "no project config (.gz-git.yaml) in current directory tree",
		}}
	}

	return []CheckResult{{
		Name:     "project-config",
		Category: CategoryConfig,
		Status:   StatusOK,
		Message:  fmt.Sprintf("project config: %s", projectConfigPath),
	}}
}

// --- Auth Checks ---

func checkTokenSources() []CheckResult {
	loader, err := config.NewLoader()
	if err != nil {
		return []CheckResult{{
			Name:     "token-source",
			Category: CategoryAuth,
			Status:   StatusWarning,
			Message:  "cannot load config for token source check",
			Detail:   err.Error(),
		}}
	}
	if err := loader.Load(); err != nil {
		return []CheckResult{{
			Name:     "token-source",
			Category: CategoryAuth,
			Status:   StatusWarning,
			Message:  "cannot load config layers for token source check",
			Detail:   err.Error(),
		}}
	}
	eff, err := loader.ResolveConfig(nil)
	if err != nil {
		return []CheckResult{{
			Name:     "token-source",
			Category: CategoryAuth,
			Status:   StatusWarning,
			Message:  "cannot resolve effective token",
			Detail:   err.Error(),
		}}
	}
	src := eff.GetSource("token")
	if eff.Token == "" {
		return []CheckResult{{
			Name:     "token-source",
			Category: CategoryAuth,
			Status:   StatusWarning,
			Message:  "no forge token resolved (env | keychain | config | none)",
			Detail:   "set GITHUB_TOKEN/GITLAB_TOKEN/GITEA_TOKEN/GZ_GIT_TOKEN or: gz-git config token set <provider> <token>",
		}}
	}
	if src == "" {
		src = "unknown"
	}
	return []CheckResult{{
		Name:     "token-source",
		Category: CategoryAuth,
		Status:   StatusOK,
		Message:  fmt.Sprintf("forge token source: %s", src),
	}}
}

func checkSSHKeys() []CheckResult {
	manager, err := config.NewManager()
	if err != nil {
		return nil
	}

	profiles, err := manager.ListProfiles()
	if err != nil {
		return nil
	}

	var results []CheckResult

	for _, name := range profiles {
		profile, err := manager.LoadProfile(name)
		if err != nil {
			continue
		}

		if profile.SSHKeyPath == "" {
			continue
		}

		keyPath := expandHome(profile.SSHKeyPath)

		info, err := os.Stat(keyPath)
		if os.IsNotExist(err) {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("ssh-key:%s", name),
				Category: CategoryAuth,
				Status:   StatusError,
				Message:  fmt.Sprintf("profile '%s': SSH key not found: %s", name, keyPath),
			})
			continue
		}

		if err != nil {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("ssh-key:%s", name),
				Category: CategoryAuth,
				Status:   StatusError,
				Message:  fmt.Sprintf("profile '%s': cannot access SSH key: %s", name, keyPath),
				Detail:   err.Error(),
			})
			continue
		}

		// Check permissions
		if runtime.GOOS != "windows" {
			perm := info.Mode().Perm()
			if perm&0o077 != 0 {
				results = append(results, CheckResult{
					Name:     fmt.Sprintf("ssh-key:%s", name),
					Category: CategoryAuth,
					Status:   StatusError,
					Message:  fmt.Sprintf("profile '%s': SSH key has loose permissions: %04o (must be 0600)", name, perm),
					Detail:   keyPath,
				})
				continue
			}
		}

		results = append(results, CheckResult{
			Name:     fmt.Sprintf("ssh-key:%s", name),
			Category: CategoryAuth,
			Status:   StatusOK,
			Message:  fmt.Sprintf("profile '%s': SSH key OK (%s)", name, keyPath),
		})
	}

	// Check default SSH keys if no profile-specific keys
	if len(results) == 0 {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			defaultKeys := []string{"id_ed25519", "id_rsa", "id_ecdsa"}
			found := false
			for _, keyName := range defaultKeys {
				keyPath := filepath.Join(homeDir, ".ssh", keyName)
				if _, err := os.Stat(keyPath); err == nil {
					found = true
					results = append(results, CheckResult{
						Name:     "ssh-key:default",
						Category: CategoryAuth,
						Status:   StatusOK,
						Message:  fmt.Sprintf("default SSH key found: %s", keyPath),
					})
					break
				}
			}
			if !found {
				results = append(results, CheckResult{
					Name:     "ssh-key:default",
					Category: CategoryAuth,
					Status:   StatusWarning,
					Message:  "no default SSH key found (~/.ssh/id_ed25519, id_rsa, id_ecdsa)",
				})
			}
		}
	}

	return results
}

// --- Forge Checks ---

func checkForgeConnectivity(ctx context.Context) []CheckResult {
	manager, err := config.NewManager()
	if err != nil {
		return nil
	}

	profiles, err := manager.ListProfiles()
	if err != nil {
		return nil
	}

	var results []CheckResult

	for _, name := range profiles {
		profile, err := manager.LoadProfile(name)
		if err != nil {
			continue
		}

		if profile.Provider == "" || profile.Token == "" {
			continue
		}

		p, err := createProvider(profile.Provider, profile.Token, profile.BaseURL) //nolint:contextcheck // provider constructors don't accept context
		if err != nil {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("forge:%s", name),
				Category: CategoryForge,
				Status:   StatusError,
				Message:  fmt.Sprintf("profile '%s': cannot create %s provider", name, profile.Provider),
				Detail:   err.Error(),
			})
			continue
		}

		// Validate token
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		valid, err := p.ValidateToken(checkCtx)
		cancel()

		if err != nil {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("forge:%s", name),
				Category: CategoryForge,
				Status:   StatusUnreachable,
				Message:  fmt.Sprintf("profile '%s': %s API unreachable", name, profile.Provider),
				Detail:   err.Error(),
			})
			continue
		}

		if !valid {
			results = append(results, CheckResult{
				Name:     fmt.Sprintf("forge:%s", name),
				Category: CategoryForge,
				Status:   StatusError,
				Message:  fmt.Sprintf("profile '%s': %s token is invalid or expired", name, profile.Provider),
			})
			continue
		}

		// Check rate limit
		rateCtx, rateCancel := context.WithTimeout(ctx, 10*time.Second)
		rl, err := p.GetRateLimit(rateCtx)
		rateCancel()

		msg := fmt.Sprintf("profile '%s': %s token valid", name, profile.Provider)
		if err == nil && rl != nil && rl.Limit > 0 {
			pct := float64(rl.Remaining) / float64(rl.Limit) * 100
			msg += fmt.Sprintf(" (rate limit: %d/%d, %.0f%%)", rl.Remaining, rl.Limit, pct)

			if pct < 10 {
				results = append(results, CheckResult{
					Name:     fmt.Sprintf("forge:%s", name),
					Category: CategoryForge,
					Status:   StatusWarning,
					Message:  fmt.Sprintf("profile '%s': %s rate limit low (%.0f%% remaining)", name, profile.Provider, pct),
					Detail:   fmt.Sprintf("Resets at %s", rl.Reset.Format(time.RFC3339)),
				})
				continue
			}
		}

		results = append(results, CheckResult{
			Name:     fmt.Sprintf("forge:%s", name),
			Category: CategoryForge,
			Status:   StatusOK,
			Message:  msg,
		})
	}

	return results
}

// --- Helpers ---

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
