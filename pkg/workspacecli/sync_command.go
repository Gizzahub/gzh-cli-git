// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package workspacecli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/hooks"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposync"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/reposynccli"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/templates"
)

func (f CommandFactory) newSyncCmd() *cobra.Command { //nolint:gocognit,gocyclo // CLI command setup naturally requires handling all flag/mode combinations
	var (
		configPath     string
		strategy       string
		parallel       int
		maxRetries     int
		resume         bool
		dryRun         bool
		yes            bool
		interactive    bool
		verbose        bool
		stateFile      string
		fullOutput     bool
		recursive      bool
		recursiveDepth int
		format         string
		pushAfterSync  bool
	)

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Clone/update repositories from config",
		Long: cliutil.QuickStartHelp(`  # Sync from default config (.gz-git.yaml) - auto-proceeds without prompt
  gz-git workspace sync

  # Sync from specific config
  gz-git workspace sync -c myworkspace.yaml

  # Preview without making changes
  gz-git workspace sync --dry-run

  # Ask for confirmation before executing
  gz-git workspace sync --interactive

  # Override strategy for all repos
  gz-git workspace sync --strategy pull

  # Resume interrupted sync
  gz-git workspace sync --resume --state-file state.json

Confirmation Behavior:
  By default, sync auto-proceeds after showing the preview.
  Use --interactive (-i) to be asked for confirmation before executing.
  Use --dry-run to show preview without executing.

Preview Behavior:
  Before executing, sync shows a detailed preview including:
    - Repository-level summary (clone/update/skip counts)
    - File-level changes per repo (added/modified/deleted)
    - Conflict warnings: uncommitted local changes, diverged branches

  Example preview output:
    ═══ Sync Preview (Detailed) ═══
    Total: 3 repositories
      + 1 will be cloned (new)
      ↓ 2 will be updated

    ⚠️  Warnings:
      • api-server: Local uncommitted changes may be overwritten

    Repository Details:
      + my-project (clone)
        → https://github.com/owner/my-project.git
      ↓ api-server (update)
        ⚠️  Uncommitted local changes detected
        Files: ~3 modified, -1 deleted

Config File Structure (Reference):
  strategy: reset # reset|pull|rebase|fetch
  parallel: 4
  repositories:
    - name: my-project
      url: https://github.com/owner/my-project.git
      path: ./repos/my-project  # Optional: defaults to name if omitted`),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Auto-detect config file if not specified
			if configPath == "" {
				detected, err := detectConfigFile(".")
				if err != nil {
					return fmt.Errorf("no config file specified and auto-detection failed: %w", err)
				}
				configPath = detected
				fmt.Fprintf(cmd.OutOrStdout(), "Using config: %s\n", configPath)
			}

			// Get config directory for path resolution
			absConfigPath, err := filepath.Abs(configPath)
			if err != nil {
				return fmt.Errorf("failed to resolve config path %s: %w", configPath, err)
			}
			configDir := filepath.Dir(absConfigPath)
			configFile := filepath.Base(absConfigPath)

			// Load config to check for "repositories" list (flat config format)
			loader := FileSpecLoader{}
			cfgData, err := loader.Load(ctx, configPath)
			if err != nil {
				return err
			}

			// Plan from repositories list (flat config format)
			var allActions []reposync.Action
			if len(cfgData.Plan.Input.Repos) > 0 {
				// CRITICAL: Filter out the devbox directory itself to prevent self-reset
				flatActions := createActionsFromFlatConfig(cfgData.Plan, configDir)
				allActions = append(allActions, flatActions...)
			}

			// Validate format early so we can suppress planning output for machine formats
			if err := cliutil.ValidateFormat(format, cliutil.CoreFormats); err != nil {
				return err
			}
			isMachine := cliutil.IsMachineFormat(format)

			// Use io.Discard for planning output when machine format is requested
			planOut := cmd.OutOrStdout()
			if isMachine {
				planOut = io.Discard
			}

			// Start planning spinner for TTY mode
			var planSpinnerProgram *tea.Program
			if isTerminal() && !isMachine {
				spinnerModel := newPlanningSpinnerModel()
				planSpinnerProgram = tea.NewProgram(spinnerModel)
				planOut = newPlanningProgressWriter(planSpinnerProgram, planOut)
				go planSpinnerProgram.Run() //nolint:errcheck // spinner error is non-critical and only logged by Bubble Tea internally
			}

			// Load Recursive Config (File 4 style)
			// We check if there are nested workspaces or forge sources defined

			recursiveCfg, recursiveErr := config.LoadConfigRecursive(configDir, configFile)
			if recursiveErr != nil {
				if !os.IsNotExist(recursiveErr) {
					fmt.Fprintf(planOut, "⚠️  Config warning: %v\n", recursiveErr)
				}
				recursiveCfg = nil
			}

			if recursiveCfg != nil {
				// Auto-create child configs if missing
				// This handles bootstrapping where child directories don't exist yet
				if err := ensureChildConfigs(planOut, recursiveCfg); err != nil {
					if planSpinnerProgram != nil {
						planSpinnerProgram.Send(planDoneMsg{})
						planSpinnerProgram.Wait()
					}
					return fmt.Errorf("failed to ensure child configs: %w", err)
				}

				// Discover workspaces (Hybrid mode)
				if err := config.LoadWorkspaces(configDir, recursiveCfg, config.HybridMode); err == nil {
					// Get forge actions
					forgeActions, err := planForgeWorkspaces(ctx, recursiveCfg, planOut, strategy, fullOutput)
					if err != nil {
						if planSpinnerProgram != nil {
							planSpinnerProgram.Send(planDoneMsg{})
							planSpinnerProgram.Wait()
						}
						return err
					}
					allActions = append(allActions, forgeActions...)

					// Get git workspace actions (type=git with URL)
					gitActions, err := planGitWorkspaces(ctx, recursiveCfg, configDir, planOut, strategy)
					if err != nil {
						if planSpinnerProgram != nil {
							planSpinnerProgram.Send(planDoneMsg{})
							planSpinnerProgram.Wait()
						}
						return err
					}
					allActions = append(allActions, gitActions...)

					// Get config workspace actions (type=config with sync.recursive=true)
					cfgWsActions, err := planConfigWorkspaces(ctx, recursiveCfg, configDir, planOut, strategy)
					if err != nil {
						if planSpinnerProgram != nil {
							planSpinnerProgram.Send(planDoneMsg{})
							planSpinnerProgram.Wait()
						}
						return err
					}
					allActions = append(allActions, cfgWsActions...)
				}

				// Deduplicate actions by target path (forge + config planners may overlap)
				allActions = deduplicateActions(allActions)
			}

			// Stop planning spinner
			if planSpinnerProgram != nil {
				planSpinnerProgram.Send(planDoneMsg{})
				planSpinnerProgram.Wait()
			}

			// Execute self-sync first (sync config directory itself)
			if recursiveCfg != nil {
				if err := executeSelfSync(ctx, recursiveCfg, configDir, planOut, dryRun); err != nil {
					fmt.Fprintf(planOut, "⚠️  Self-sync warning: %v\n", err)
					// Continue with workspace sync even if self-sync fails
				}
			}

			if len(allActions) == 0 {
				fmt.Println("No repositories found to sync.")
				return nil
			}

			out := cmd.OutOrStdout()

			if !isMachine {
				// --dry-run always shows full detail so user can review before deciding
				showDetail := verbose || dryRun
				if showDetail {
					detailedChanges, summary := buildDetailedSyncPreview(ctx, allActions)
					displayDetailedSyncPreview(out, detailedChanges, summary)
				} else {
					// Compact: only show count + conflict warnings
					summary := buildSyncSummary(allActions)
					displayCompactSyncPreview(ctx, out, summary, allActions)
				}

				// Determine if we need confirmation
				// Prompt only when: --interactive is set and not --dry-run and not --yes
				// Default behavior (no flags): auto-proceed without prompt
				needsConfirmation := interactive && !dryRun && !yes && isTerminal()

				if needsConfirmation {
					proceed, err := confirmSyncPrompt()
					if err != nil {
						return fmt.Errorf("confirmation prompt failed: %w", err)
					}
					if !proceed {
						fmt.Fprintln(out, "Sync canceled.")
						return nil
					}
				}

				// In dry-run mode, we already showed the summary, no need to execute
				// unless we're outputting JSON/LLM
				if dryRun {
					if recursive {
						absConfigDirDry, err := filepath.Abs(configDir)
						if err != nil {
							absConfigDirDry = configDir
						}
						rOpts := recursiveSyncOpts{
							MaxDepth: recursiveDepth,
							Visited:  map[string]bool{absConfigDirDry: true},
							Out:      out,
							Depth:    0,
							Strategy: strategy,
							DryRun:   true,
						}
						scanForChildConfigs(out, allActions, rOpts)
					}
					fmt.Fprintln(out, "\n[dry-run] No changes made.")
					return nil
				}
			}

			// Merge options
			runOpts := cfgData.Run
			if cmd.Flags().Changed("parallel") {
				runOpts.Parallel = parallel
			}
			if cmd.Flags().Changed("max-retries") {
				runOpts.MaxRetries = maxRetries
			}
			if cmd.Flags().Changed("resume") {
				runOpts.Resume = resume
			}
			if cmd.Flags().Changed("dry-run") {
				runOpts.DryRun = dryRun
			}
			if runOpts.Resume && stateFile == "" {
				return fmt.Errorf("resume requested but no --state-file provided")
			}

			// If recursive config has parallel setting and flag not set
			if recursiveCfg != nil && recursiveCfg.GetParallel() > 0 && !cmd.Flags().Changed("parallel") {
				runOpts.Parallel = recursiveCfg.GetParallel()
			}

			// Use precomputed planner (same as from-config logic)
			staticPlanner := &precomputedPlanner{actions: allActions}
			exec := reposync.GitExecutor{}
			var state reposync.StateStore
			if stateFile != "" {
				state = reposync.NewFileStateStore(stateFile)
			} else {
				state = reposync.NewInMemoryStateStore()
			}

			var execResult reposync.ExecutionResult
			var durationMs int64

			//nolint:gocritic // ifElseChain: three distinct bool conditions cannot be expressed as a switch
			if isMachine {
				// No progress display in machine output mode
				orch := reposync.NewOrchestrator(staticPlanner, exec, state)
				startTime := time.Now()
				var orchErr error
				execResult, orchErr = orch.Run(ctx, reposync.RunRequest{
					RunOptions: runOpts,
					Progress:   &reposync.NoopProgressSink{},
					State:      state,
				})
				durationMs = time.Since(startTime).Milliseconds()
				if orchErr != nil {
					return fmt.Errorf("workspace sync failed: %w", orchErr)
				}
			} else if isTerminal() {
				// Bubble Tea TUI with alternate screen
				tuiResult, tuiErr := runSyncTUI(ctx, out, allActions, runOpts, state, staticPlanner, exec)
				if tuiErr != nil {
					return tuiErr
				}
				execResult = tuiResult.ExecResult
				durationMs = tuiResult.Duration.Milliseconds()
			} else {
				// Non-TTY: simple line-by-line progress
				orch := reposync.NewOrchestrator(staticPlanner, exec, state)
				startTime := time.Now()
				var orchErr error
				execResult, orchErr = orch.Run(ctx, reposync.RunRequest{
					RunOptions: runOpts,
					Progress:   &consoleProgress{out: cmd.OutOrStdout()},
					State:      state,
				})
				durationMs = time.Since(startTime).Milliseconds()
				if orchErr != nil {
					return fmt.Errorf("workspace sync failed: %w", orchErr)
				}
			}
			var participationErr error
			if !dryRun {
				if err := recordReadOnlyWorkspaceAccess(ctx, execResult, configDir); err != nil {
					return err
				}
				participationErr = reconcileIntegrationParticipation(ctx, execResult, configDir)
			}

			// Display machine output if requested
			switch format {
			case "json":
				if err := displaySyncResultsJSON(out, execResult, durationMs, verbose); err != nil {
					return fmt.Errorf("failed to display JSON output: %w", err)
				}
				// Skip recursive child diff printing for machine format for now
				if participationErr != nil {
					return fmt.Errorf("workspace integration participation reconcile failed: %w", participationErr)
				}
				return errSyncPartialFailure(execResult)
			case "llm":
				if err := displaySyncResultsLLM(out, execResult, durationMs); err != nil {
					return fmt.Errorf("failed to display LLM output: %w", err)
				}
				// Skip recursive child diff printing for machine format for now
				if participationErr != nil {
					return fmt.Errorf("workspace integration participation reconcile failed: %w", participationErr)
				}
				return errSyncPartialFailure(execResult)
			}
			if participationErr != nil {
				return fmt.Errorf("workspace integration participation reconcile failed: %w", participationErr)
			}

			// Recursive child workspace sync
			if recursive {
				absConfigDir, err := filepath.Abs(configDir)
				if err != nil {
					absConfigDir = configDir
				}
				rOpts := recursiveSyncOpts{
					MaxDepth: recursiveDepth,
					Visited:  map[string]bool{absConfigDir: true},
					Out:      out,
					Depth:    0,
					Strategy: strategy,
					Parallel: runOpts.Parallel,
					DryRun:   false,
				}
				syncChildWorkspaces(ctx, execResult, rOpts)
			}

			// Build and display workspace hierarchy tree
			tree := buildResultTree(execResult, filepath.Base(configDir))
			if tree != nil && len(tree.Children) > 0 {
				fmt.Fprintln(out, "\n── Workspace Hierarchy ──")
				renderTree(out, tree)
			}

			// Post-sync push: push repos that synced successfully
			if pushAfterSync && !dryRun {
				pushTargets := collectPushTargets(execResult)
				if len(pushTargets) > 0 {
					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "─── Post-sync push ────────────────────────────────────────────")
					pushResults := executePushForSyncedRepos(ctx, pushTargets, runOpts.Parallel)
					displayPushResults(out, pushResults, isMachine)
				}
			}

			return errSyncPartialFailure(execResult)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file (auto-detects "+DefaultConfigFile+")")
	cmd.Flags().StringVar(&strategy, "strategy", "", "Strategy override (reset|pull|rebase|fetch)")
	cmd.Flags().IntVar(&parallel, "parallel", 0, "Parallel workers (overrides config)")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 0, "Retry attempts per repo (overrides config)")
	cmd.Flags().BoolVar(&resume, "resume", false, "Resume from previous state")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without making changes")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt (auto-approve, deprecated: now default behavior)")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Ask for confirmation before executing (default: auto-proceed)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed repository preview (file changes, conflicts)")
	cmd.Flags().StringVar(&stateFile, "state-file", "", "Path to persist run state for resume")
	cmd.Flags().BoolVar(&fullOutput, "full", false, "Output all fields (name, path) even if redundant")
	cmd.Flags().BoolVar(&recursive, "recurse-workspaces", false, "Recursively sync child workspaces containing "+DefaultConfigFile)
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "deprecated: use --recurse-workspaces")
	if err := cmd.Flags().MarkDeprecated("recursive", "use --recurse-workspaces instead"); err != nil {
		panic(err)
	}
	cmd.Flags().IntVar(&recursiveDepth, "recursive-depth", 3, "Maximum recursion depth for --recurse-workspaces")
	cmd.Flags().StringVar(&format, "format", "default", "Output format (default, compact, json, llm)")
	cmd.Flags().BoolVar(&pushAfterSync, "push", false, "Push after successful sync (only repos with local commits ahead)")

	return cmd
}

func recordReadOnlyWorkspaceAccess(ctx context.Context, result reposync.ExecutionResult, configDir string) error {
	for _, actionResult := range result.Succeeded {
		path := actionResult.Action.Repo.TargetPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}
		var err error
		if actionResult.Action.ReadOnly {
			err = config.RecordWorkspaceReadOnly(ctx, path)
		} else {
			err = config.ClearWorkspaceAccessMarker(ctx, path)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// reconcileIntegrationParticipation applies repo-root branch.integrationBranch
// declarations only after the repository action itself succeeded. Its separate
// error path makes a policy/configuration failure distinguishable from sync.
func reconcileIntegrationParticipation(ctx context.Context, result reposync.ExecutionResult, configDir string) error {
	var errs []error
	for _, actionResult := range result.Succeeded {
		if actionResult.Action.Type != reposync.ActionClone && actionResult.Action.Type != reposync.ActionUpdate {
			continue
		}
		path := actionResult.Action.Repo.TargetPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}
		if _, err := config.ReconcileIntegrationParticipation(ctx, path); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

// createActionsFromFlatConfig converts flat config PlanRequest to Actions.
// configDir is the directory containing the config file, used to filter out devbox itself.
func createActionsFromFlatConfig(req reposync.PlanRequest, configDir string) []reposync.Action {
	defaultStrategy := req.Options.DefaultStrategy
	if defaultStrategy == "" {
		defaultStrategy = reposync.StrategyReset
	}

	// Get absolute path of config directory to compare with target paths
	absConfigDir, err := filepath.Abs(configDir)
	if err != nil {
		absConfigDir = configDir
	}

	actions := make([]reposync.Action, 0, len(req.Input.Repos))
	for _, repo := range req.Input.Repos {
		// CRITICAL: Skip if target path resolves to the devbox directory itself
		// This prevents workspace sync from resetting the devbox repository
		absTargetPath, err := filepath.Abs(repo.TargetPath)
		if err != nil {
			absTargetPath = repo.TargetPath
		}
		if absTargetPath == absConfigDir {
			// Skip devbox directory itself
			continue
		}

		strategy := repo.Strategy
		if strategy == "" {
			strategy = defaultStrategy
		}

		// Use ActionUpdate - executor handles clone if directory doesn't exist
		actions = append(actions, reposync.Action{
			Repo:      repo,
			Type:      reposync.ActionUpdate,
			Strategy:  strategy,
			Reason:    "workspace config",
			PlannedBy: "config",
		})
	}
	return actions
}

// planForgeWorkspaces generates actions from recursive config workspaces.
func planForgeWorkspaces(ctx context.Context, cfg *config.Config, out io.Writer, strategyOverrideStr string, fullOutput bool) ([]reposync.Action, error) { //nolint:gocognit,gocyclo // forge workspace planning requires handling many config combinations
	workspaces := config.GetForgeWorkspaces(cfg)
	if len(workspaces) == 0 {
		return nil, nil
	}

	fmt.Fprintf(out, "Found %d recursive workspaces\n", len(workspaces))

	var strategyOverride reposync.Strategy
	if strategyOverrideStr != "" {
		s, err := reposync.ParseStrategy(strategyOverrideStr)
		if err != nil {
			return nil, err
		}
		strategyOverride = s
	}

	// Get config directory for resolving paths
	configDir := filepath.Dir(cfg.ConfigPath)

	var allActions []reposync.Action

	for name, ws := range workspaces {
		if ws.Source == nil {
			continue
		}

		// Default path to workspace name when omitted (compact config convention)
		effectivePath := ws.Path
		if effectivePath == "" {
			effectivePath = name
		}

		// Resolve workspace path
		wsPath := resolveWorkspacePath(effectivePath, configDir)

		// Skip workspaces that point to the config directory itself
		if wsPath == configDir || effectivePath == "." {
			fmt.Fprintf(out, "⚠️  Skipping workspace '%s': path '%s' points to config directory (self-sync not allowed)\n", name, effectivePath)
			continue
		}

		fmt.Fprintf(out, "Planning nested workspace '%s' (%s/%s)...\n", name, ws.Source.Provider, ws.Source.Org)

		// Merge root and workspace hooks
		mergedHooks := hooks.Merge(cfg.Hooks, ws.Hooks)

		// Execute before hooks
		if mergedHooks != nil && len(mergedHooks.Before) > 0 {
			// Ensure parent directory exists for before hooks
			if err := os.MkdirAll(filepath.Dir(wsPath), 0o750); err != nil {
				return nil, fmt.Errorf("failed to create directory for workspace '%s' before hooks: %w", name, err)
			}

			fmt.Fprintf(out, "  → Running before hooks for workspace '%s'...\n", name)
			if err := hooks.Execute(ctx, mergedHooks, "before", filepath.Dir(wsPath), nil); err != nil {
				return nil, fmt.Errorf("before hooks failed for workspace '%s': %w", name, err)
			}
		}

		prov, err := createProviderFromSource(ws.Source, ws, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create provider for workspace '%s': %w", name, err)
		}

		// Resolve values with profile fallback
		cloneProto := ws.CloneProto
		sshPort := ws.SSHPort
		token := ws.Source.Token

		// Fallback to profile values if not set
		if ws.Profile != "" && cfg != nil {
			profile := config.GetProfileFromChain(cfg, ws.Profile)
			if profile != nil {
				if cloneProto == "" {
					cloneProto = profile.CloneProto
				}
				if sshPort == 0 {
					sshPort = profile.SSHPort
				}
				if token == "" {
					token = profile.Token
				}
			}
		}

		// Fallback to root config's active profile
		if cfg != nil && cfg.Profile != "" {
			profile := config.GetProfileFromChain(cfg, cfg.Profile)
			if profile != nil {
				if cloneProto == "" {
					cloneProto = profile.CloneProto
				}
				if sshPort == 0 {
					sshPort = profile.SSHPort
				}
				if token == "" {
					token = profile.Token
				}
			}
		}

		// Fallback to root config direct values
		if cloneProto == "" && cfg != nil {
			cloneProto = cfg.GetCloneProto()
		}
		if sshPort == 0 && cfg != nil {
			sshPort = cfg.GetSSHPort()
		}
		if token == "" && cfg != nil {
			token = cfg.Token
		}

		// Extract branch from workspace config, falling back to root config
		branch := ""
		if ws.Branch != nil && len(ws.Branch.DefaultBranch) > 0 {
			branch = strings.Join(ws.Branch.DefaultBranch, ",")
		} else if cfg.Branch != nil && len(cfg.Branch.DefaultBranch) > 0 {
			branch = strings.Join(cfg.Branch.DefaultBranch, ",")
		}

		includePatterns, excludePatterns := effectiveForgeWorkspacePatterns(cfg, ws)

		plannerConfig := reposync.ForgePlannerConfig{
			TargetPath:            wsPath,
			Organization:          ws.Source.Org,
			IncludeSubgroups:      ws.Source.IncludeSubgroups,
			SubgroupMode:          ws.Source.SubgroupMode,
			IncludePrivate:        true, // workspace sync should include private/internal repos
			Branch:                branch,
			FilterIncludePatterns: includePatterns,
			FilterExcludePatterns: excludePatterns,
			Auth: reposync.AuthConfig{
				Token:    token,
				Provider: ws.Source.Provider,
				SSHPort:  sshPort,
			},
			CloneProto: cloneProto,
			SSHPort:    sshPort,
		}

		planner := reposync.NewForgePlanner(prov, plannerConfig)
		planReq := reposync.PlanRequest{
			Options: reposync.PlanOptions{
				DefaultStrategy: workspaceSyncStrategy(ws, strategyOverride),
			},
		}

		if planReq.Options.DefaultStrategy == "" {
			planReq.Options.DefaultStrategy = reposync.StrategyReset
		}

		// Enable orphan cleanup if configured
		if ws.Sync != nil && ws.Sync.CleanupOrphans {
			planReq.Options.CleanupOrphans = true
			planReq.Options.Roots = []string{wsPath}
		}

		plan, err := planner.Plan(ctx, planReq)
		if err != nil {
			return nil, fmt.Errorf("failed to plan workspace '%s': %w", name, err)
		}
		if ws.Access.IsReadOnly() {
			markActionsReadOnly(plan.Actions)
		}

		fmt.Fprintf(out, "  → Found %d repositories\n", len(plan.Actions))

		// Handle config: either create symlink (configLink) or write child config
		if ws.ConfigLink != "" {
			// Create symlink instead of writing config
			fmt.Fprintf(out, "  → Creating config symlink: %s → %s\n", filepath.Join(wsPath, ".gz-git.yaml"), ws.ConfigLink)
			if err := config.CreateConfigSymlink(ws.ConfigLink, wsPath, configDir); err != nil {
				return nil, fmt.Errorf("failed to create config symlink for '%s': %w", name, err)
			}
		} else {
			// Write child config with repository list
			if err := writeChildForgeConfig(out, cfg, ws, wsPath, plan.Actions, cloneProto, sshPort, fullOutput); err != nil {
				return nil, fmt.Errorf("failed to write child config for '%s': %w", name, err)
			}
		}

		// Execute after hooks (run after config is set up but before actual sync)
		// Note: Ideally these should run after the sync completes, but the current
		// architecture separates planning from execution. For now, we run them
		// after the workspace is prepared (directory created, config set up).
		if mergedHooks != nil && len(mergedHooks.After) > 0 {
			fmt.Fprintf(out, "  → Running after hooks for workspace '%s'...\n", name)
			if err := hooks.Execute(ctx, mergedHooks, "after", wsPath, nil); err != nil {
				return nil, fmt.Errorf("after hooks failed for workspace '%s': %w", name, err)
			}
		}

		for i := range plan.Actions {
			plan.Actions[i].Workspace = name
		}
		allActions = append(allActions, plan.Actions...)
	}

	return allActions, nil
}

func effectiveForgeWorkspacePatterns(cfg *config.Config, ws *config.Workspace) (includePatterns, excludePatterns []string) {
	if cfg != nil {
		includePatterns = cfg.GetIncludePatterns()
		excludePatterns = cfg.GetExcludePatterns()
	}
	if len(ws.IncludePatterns) > 0 {
		includePatterns = ws.IncludePatterns
	}
	if len(ws.ExcludePatterns) > 0 {
		excludePatterns = ws.ExcludePatterns
	}

	return includePatterns, excludePatterns
}

// planGitWorkspaces generates actions from git workspaces (type=git with URL).
func planGitWorkspaces(ctx context.Context, cfg *config.Config, configDir string, out io.Writer, strategyOverrideStr string) ([]reposync.Action, error) { //nolint:gocognit,gocyclo // git workspace planning requires handling hooks, branches, strategies, and config inheritance
	workspaces := config.GetGitWorkspaces(cfg)
	if len(workspaces) == 0 {
		return nil, nil
	}

	fmt.Fprintf(out, "Found %d git workspaces\n", len(workspaces))

	var strategyOverride reposync.Strategy
	if strategyOverrideStr != "" {
		s, err := reposync.ParseStrategy(strategyOverrideStr)
		if err != nil {
			return nil, err
		}
		strategyOverride = s
	}

	var allActions []reposync.Action

	for name, ws := range workspaces {
		// Default path to workspace name when omitted (compact config convention)
		effectivePath := ws.Path
		if effectivePath == "" {
			effectivePath = name
		}

		// Resolve workspace path
		wsPath := resolveWorkspacePath(effectivePath, configDir)

		// Skip workspaces that point to the config directory itself
		// This prevents accidental reset of the devbox/orchestrator directory
		if wsPath == configDir || effectivePath == "." {
			fmt.Fprintf(out, "⚠️  Skipping workspace '%s': path '%s' points to config directory (self-sync not allowed)\n", name, effectivePath)
			continue
		}

		fmt.Fprintf(out, "Planning git workspace '%s' (%s)...\n", name, ws.URL)

		// Merge root and workspace hooks
		mergedHooks := hooks.Merge(cfg.Hooks, ws.Hooks)

		// Execute before hooks
		if mergedHooks != nil && len(mergedHooks.Before) > 0 {
			// Ensure parent directory exists for before hooks
			if err := os.MkdirAll(filepath.Dir(wsPath), 0o750); err != nil {
				return nil, fmt.Errorf("failed to create directory for workspace '%s' before hooks: %w", name, err)
			}

			fmt.Fprintf(out, "  → Running before hooks for workspace '%s'...\n", name)
			if err := hooks.Execute(ctx, mergedHooks, "before", filepath.Dir(wsPath), nil); err != nil {
				return nil, fmt.Errorf("before hooks failed for workspace '%s': %w", name, err)
			}
		}

		// Create configLink symlink if specified.
		// configLink 명시 = 사용자의 명시적 의도이므로, 기존 파일이 user-maintained여도
		// 경고만 출력하고 symlink로 교체한다 (CreateConfigSymlinkForce 사용).
		if ws.ConfigLink != "" {
			childCfgFile := filepath.Join(wsPath, ".gz-git.yaml")
			isSymlink, symlinkErr := config.IsConfigSymlink(childCfgFile)
			if symlinkErr != nil {
				isSymlink = false // treat as non-symlink on error
			}
			if !isSymlink {
				if _, statErr := os.Stat(childCfgFile); statErr == nil {
					fmt.Fprintf(out, "  ⚠ workspace '%s': replacing existing config with configLink symlink: %s\n", name, childCfgFile)
				}
			}
			fmt.Fprintf(out, "  → Creating config symlink: %s → %s\n", childCfgFile, ws.ConfigLink)
			if err := config.CreateConfigSymlinkForce(ws.ConfigLink, wsPath, configDir); err != nil {
				return nil, fmt.Errorf("failed to create config symlink for '%s': %w", name, err)
			}
		}

		// Determine strategy
		strategy := workspaceSyncStrategy(ws, strategyOverride)
		if strategy == "" {
			strategy = reposync.StrategyReset
		}

		// Extract branch from workspace config, falling back to root config
		branch := ""
		if ws.Branch != nil && len(ws.Branch.DefaultBranch) > 0 {
			branch = strings.Join(ws.Branch.DefaultBranch, ",")
		} else if cfg.Branch != nil && len(cfg.Branch.DefaultBranch) > 0 {
			branch = strings.Join(cfg.Branch.DefaultBranch, ",")
		}

		// Create action for this git workspace
		action := reposync.Action{
			Repo: reposync.RepoSpec{
				Name:              name,
				CloneURL:          ws.URL,
				AdditionalRemotes: ws.AdditionalRemotes,
				TargetPath:        wsPath,
				Branch:            branch,
			},
			Type:      reposync.ActionUpdate,
			Strategy:  strategy,
			ReadOnly:  ws.Access.IsReadOnly(),
			Reason:    "git workspace",
			PlannedBy: "config",
			Workspace: name,
		}

		allActions = append(allActions, action)

		// Execute after hooks
		if mergedHooks != nil && len(mergedHooks.After) > 0 {
			// Ensure workspace directory exists for after hooks
			if err := os.MkdirAll(wsPath, 0o750); err != nil {
				return nil, fmt.Errorf("failed to create workspace directory '%s' for after hooks: %w", name, err)
			}

			fmt.Fprintf(out, "  → Running after hooks for workspace '%s'...\n", name)
			if err := hooks.Execute(ctx, mergedHooks, "after", wsPath, nil); err != nil {
				return nil, fmt.Errorf("after hooks failed for workspace '%s': %w", name, err)
			}
		}
	}

	return allActions, nil
}

func workspaceSyncStrategy(ws *config.Workspace, override reposync.Strategy) reposync.Strategy {
	if ws.Access.IsReadOnly() {
		return reposync.StrategyPull
	}
	if override != "" {
		return override
	}
	if ws.Sync != nil && ws.Sync.Strategy != "" {
		strategy, err := reposync.ParseStrategy(ws.Sync.Strategy)
		if err == nil {
			return strategy
		}
	}
	return ""
}

// planConfigWorkspaces loads child configs from type=config workspaces with sync.recursive=true
// and returns their repository actions for inclusion in the parent's execution plan.
func planConfigWorkspaces(ctx context.Context, cfg *config.Config, configDir string, out io.Writer, strategyOverrideStr string) ([]reposync.Action, error) { //nolint:gocognit // config workspace planning requires loading and recursing into child configs
	workspaces := config.GetConfigWorkspaces(cfg)
	if len(workspaces) == 0 {
		return nil, nil
	}

	fmt.Fprintf(out, "Found %d recursive config workspaces\n", len(workspaces))

	var allActions []reposync.Action

	for name, ws := range workspaces {
		// Default path to workspace name when omitted
		effectivePath := ws.Path
		if effectivePath == "" {
			effectivePath = name
		}

		// Resolve workspace path
		wsPath := resolveWorkspacePath(effectivePath, configDir)

		// Skip workspaces that point to the config directory itself
		if wsPath == configDir || effectivePath == "." {
			fmt.Fprintf(out, "⚠️  Skipping workspace '%s': path '%s' points to config directory\n", name, effectivePath)
			continue
		}

		childConfigPath := filepath.Join(wsPath, DefaultConfigFile)

		// Check if child config exists (should have been created by ensureChildConfigs/configLink)
		if _, err := os.Stat(childConfigPath); os.IsNotExist(err) {
			fmt.Fprintf(out, "⚠️  Skipping workspace '%s': no config at %s\n", name, childConfigPath)
			continue
		}

		fmt.Fprintf(out, "Planning config workspace '%s' (%s)...\n", name, childConfigPath)

		// Load flat config from child
		loader := FileSpecLoader{}
		cfgData, err := loader.Load(ctx, childConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config for workspace '%s': %w", name, err)
		}

		// Build actions from child's repositories list
		if len(cfgData.Plan.Input.Repos) > 0 {
			// Apply strategy override if specified
			if strategyOverrideStr != "" {
				s, sErr := reposync.ParseStrategy(strategyOverrideStr)
				if sErr == nil {
					cfgData.Plan.Options.DefaultStrategy = s
				}
			}
			if ws.Access.IsReadOnly() {
				cfgData.Plan.Options.DefaultStrategy = reposync.StrategyPull
			}

			childActions := createActionsFromFlatConfig(cfgData.Plan, wsPath)
			if ws.Access.IsReadOnly() {
				markActionsReadOnly(childActions)
			}
			for i := range childActions {
				childActions[i].Workspace = name
			}
			fmt.Fprintf(out, "  → Found %d repositories\n", len(childActions))
			allActions = append(allActions, childActions...)
		}

		// Also load recursive/hierarchical config from child
		childConfigFile := filepath.Base(childConfigPath)
		childRecursiveCfg, recursiveErr := config.LoadConfigRecursive(wsPath, childConfigFile)
		if recursiveErr != nil && !os.IsNotExist(recursiveErr) {
			fmt.Fprintf(out, "  ⚠️  Config warning: %v\n", recursiveErr)
		}

		if childRecursiveCfg != nil {
			if err := config.LoadWorkspaces(wsPath, childRecursiveCfg, config.HybridMode); err == nil {
				forgeActions, fErr := planForgeWorkspaces(ctx, childRecursiveCfg, out, strategyOverrideStr, false)
				if fErr != nil {
					return nil, fmt.Errorf("failed to plan forge workspaces in '%s': %w", name, fErr)
				}
				if ws.Access.IsReadOnly() {
					markActionsReadOnly(forgeActions)
				}
				allActions = append(allActions, forgeActions...)

				gitActions, gErr := planGitWorkspaces(ctx, childRecursiveCfg, wsPath, out, strategyOverrideStr)
				if gErr != nil {
					return nil, fmt.Errorf("failed to plan git workspaces in '%s': %w", name, gErr)
				}
				if ws.Access.IsReadOnly() {
					markActionsReadOnly(gitActions)
				}
				allActions = append(allActions, gitActions...)
			}
		}
	}

	return allActions, nil
}

func markActionsReadOnly(actions []reposync.Action) {
	for i := range actions {
		actions[i].ReadOnly = true
		actions[i].Strategy = reposync.StrategyPull
	}
}

// writeChildForgeConfig writes a child config file with the list of repositories.
// It respects the workspace's ChildConfigMode and protects user-maintained files.
func writeChildForgeConfig(out io.Writer, parentCfg *config.Config, ws *config.Workspace, wsPath string, actions []reposync.Action, cloneProto string, sshPort int, fullOutput bool) error {
	if len(actions) == 0 {
		return nil
	}

	// Get child config mode (workspace > config > default)
	mode := ws.ChildConfigMode
	if mode == "" && parentCfg != nil && parentCfg.ChildConfigMode != "" {
		mode = parentCfg.ChildConfigMode
	}
	mode = mode.Default()

	// If mode is "none": create directory only, no config file
	if mode == config.ChildConfigModeNone {
		if err := os.MkdirAll(wsPath, 0o750); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
		fmt.Fprintf(out, "  → Created directory %s (no config, childConfigMode: none)\n", wsPath)
		return nil
	}

	childConfigPath := filepath.Join(wsPath, ".gz-git.yaml")

	// Detect existing config format to protect user-maintained files
	format, err := config.DetectChildConfigFormat(childConfigPath)
	if err != nil {
		return fmt.Errorf("detect config format: %w", err)
	}

	// If user-maintained: warn and skip (don't overwrite)
	if format == config.ChildConfigFormatUserMaintained {
		fmt.Fprintf(out, "  ⚠ Skipping %s (user-maintained, use --force to overwrite)\n", childConfigPath)
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(wsPath, 0o750); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Generate config based on mode
	switch mode {
	case config.ChildConfigModeWorkspaces:
		return writeWorkspacesFormatConfig(out, parentCfg, ws, actions, childConfigPath)
	case config.ChildConfigModeNone:
		// Already handled above before directory creation; should not reach here.
		return nil
	case config.ChildConfigModeRepositories:
		return writeRepositoriesFormatConfig(out, parentCfg, ws, actions, childConfigPath, cloneProto, sshPort, fullOutput)
	default:
		// unknown/empty mode: fall back to repositories format
		return writeRepositoriesFormatConfig(out, parentCfg, ws, actions, childConfigPath, cloneProto, sshPort, fullOutput)
	}
}

// writeRepositoriesFormatConfig writes a child config in repositories array format.
func writeRepositoriesFormatConfig(out io.Writer, parentCfg *config.Config, ws *config.Workspace, actions []reposync.Action, childConfigPath, cloneProto string, sshPort int, fullOutput bool) error {
	// Build repository list from actions
	repos := make([]templates.ChildForgeRepoData, 0, len(actions))
	for _, action := range actions {
		// Calculate relative path from workspace root
		relPath := action.Repo.Name
		if action.Repo.TargetPath != "" {
			// Extract just the repo directory name from full path
			relPath = filepath.Base(action.Repo.TargetPath)
		}

		// Omit path if it equals name (compact output)
		// Path defaults to Name when loading config, so redundant paths can be omitted
		pathOutput := relPath
		if !fullOutput && relPath == action.Repo.Name {
			pathOutput = ""
		}

		repos = append(repos, templates.ChildForgeRepoData{
			Name:   action.Repo.Name,
			URL:    action.Repo.CloneURL,
			Path:   pathOutput,
			Branch: action.Repo.Branch,
		})
	}

	// Determine strategy string
	strategyStr := "pull"
	if ws.Sync != nil && ws.Sync.Strategy != "" {
		strategyStr = ws.Sync.Strategy
	}

	// Determine parallel
	parallel := repository.DefaultLocalParallel
	if ws.Parallel > 0 {
		parallel = ws.Parallel
	} else if parentCfg != nil && parentCfg.GetParallel() > 0 {
		parallel = parentCfg.GetParallel()
	}

	data := templates.ChildForgeData{
		Parent:       parentCfg.ConfigPath,
		Profile:      ws.Profile,
		GeneratedAt:  time.Now().Format(time.RFC3339),
		Provider:     ws.Source.Provider,
		Organization: ws.Source.Org,
		Strategy:     strategyStr,
		Parallel:     parallel,
		CloneProto:   cloneProto,
		SSHPort:      sshPort,
		Repositories: repos,
	}

	content, err := templates.Render(templates.RepositoriesChildForge, data)
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	if err := os.WriteFile(childConfigPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Fprintf(out, "  → Updated %s (%d repositories)\n", childConfigPath, len(repos))
	return nil
}

// writeWorkspacesFormatConfig writes a child config in workspaces map format.
func writeWorkspacesFormatConfig(out io.Writer, parentCfg *config.Config, ws *config.Workspace, actions []reposync.Action, childConfigPath string) error {
	// Build workspace entries from actions
	workspaces := make([]templates.ChildWorkspaceEntry, 0, len(actions))
	for _, action := range actions {
		// Calculate relative path from workspace root
		relPath := action.Repo.Name
		if action.Repo.TargetPath != "" {
			relPath = filepath.Base(action.Repo.TargetPath)
		}

		workspaces = append(workspaces, templates.ChildWorkspaceEntry{
			Name: action.Repo.Name,
			Path: relPath,
		})
	}

	data := templates.ChildWorkspacesData{
		Parent:       parentCfg.ConfigPath,
		Profile:      ws.Profile,
		GeneratedAt:  time.Now().Format(time.RFC3339),
		Provider:     ws.Source.Provider,
		Organization: ws.Source.Org,
		Workspaces:   workspaces,
	}

	content, err := templates.Render(templates.WorkspacesChildForge, data)
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	if err := os.WriteFile(childConfigPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Fprintf(out, "  → Updated %s (%d workspaces, map format)\n", childConfigPath, len(workspaces))
	return nil
}

// createProviderFromSource delegates to reposynccli.CreateProviderFromSource.
// This function is kept for backwards compatibility within this package.
func createProviderFromSource(src *config.ForgeSource, ws *config.Workspace, cfg *config.Config) (reposync.ForgeProvider, error) {
	return reposynccli.CreateProviderFromSource(src, ws, cfg)
}

// Helper types.
type precomputedPlanner struct {
	actions []reposync.Action
}

func (p *precomputedPlanner) Plan(_ context.Context, _ reposync.PlanRequest) (reposync.Plan, error) {
	return reposync.Plan{Actions: p.actions}, nil
}

func (p *precomputedPlanner) Describe(_ reposync.PlanRequest) string {
	return fmt.Sprintf("precomputed plan with %d actions", len(p.actions))
}

// consoleProgress implements reposync.ProgressSink for console output.
type consoleProgress struct {
	out io.Writer
}

// OnStart implements reposync.ProgressSink.
func (p *consoleProgress) OnStart(action reposync.Action) {
	fmt.Fprintf(p.out, "→ [%s] %s (%s)\n", action.Type, action.Repo.Name, action.Repo.TargetPath)
}

// OnProgress implements reposync.ProgressSink.
func (p *consoleProgress) OnProgress(action reposync.Action, message string, progress float64) {
	// Suppress redundant retry messages for non-git-repo errors to reduce noise
	// These get summarized in OnComplete as a final error
	if strings.Contains(message, "retrying") {
		return
	}
	fmt.Fprintf(p.out, "   [%s] %s: %s (%.0f%%)\n", action.Type, action.Repo.Name, message, progress*100)
}

// OnComplete implements reposync.ProgressSink.
func (p *consoleProgress) OnComplete(result reposync.ActionResult) {
	if result.Error != nil {
		fmt.Fprintf(p.out, "✗ [%s] %s: %v\n", result.Action.Type, result.Action.Repo.Name, result.Error)
		return
	}
	msg := result.Message
	if compact := reposync.FormatCompactStatus(result.PostStatus); compact != "" {
		msg = compact
	}
	fmt.Fprintf(p.out, "✓ [%s] %s: %s\n", result.Action.Type, result.Action.Repo.Name, msg)
}

// ensureChildConfigs checks explicit workspaces and creates .gz-git.yaml if missing.
func ensureChildConfigs(out io.Writer, cfg *config.Config) error { //nolint:gocognit // workspace bootstrapping requires checking multiple config modes, symlinks, and file states
	if cfg == nil || len(cfg.Workspaces) == 0 {
		return nil
	}

	for name, ws := range cfg.Workspaces {
		// Skip workspaces with forge source - they are handled by planForgeWorkspaces
		// which writes a complete config with repository list
		if ws.Source != nil {
			continue
		}

		// Skip git-type workspaces (leaf repos with URL) - they don't need a child config.
		// These are single git repositories that will be cloned/updated directly.
		// Creating .gz-git.yaml inside them would be incorrect (they are leaf nodes).
		effectiveType := ws.Type.Resolve(ws.Source != nil)
		if effectiveType == config.WorkspaceTypeGit && ws.URL != "" {
			continue
		}

		// Get child config mode (workspace > config > default)
		mode := ws.ChildConfigMode
		if mode == "" && cfg.ChildConfigMode != "" {
			mode = cfg.ChildConfigMode
		}
		// Skip if mode is "none"
		if mode == config.ChildConfigModeNone {
			continue
		}

		// We only care about explicit workspaces defined in this config
		// If it's type: config (explicitly set or inferred)
		// Or if it's meant to be a workspace but directory/config is missing

		// Resolve path relative to config file's directory.
		// resolveWorkspacePath handles absolute, home-relative (~/) and relative paths.
		configDir := filepath.Dir(cfg.ConfigPath)
		wsPath := resolveWorkspacePath(ws.Path, configDir)

		// If configLink is set, create symlink instead of bootstrap config.
		// configLink 명시 = 사용자의 명시적 의도이므로, 기존 파일이 user-maintained여도
		// 에러로 중단하지 않고 경고만 출력한 뒤 symlink로 교체한다.
		if ws.ConfigLink != "" {
			childConfigFile := filepath.Join(wsPath, ".gz-git.yaml")
			isSymlink, symlinkErr := config.IsConfigSymlink(childConfigFile)
			if symlinkErr != nil {
				isSymlink = false // treat as non-symlink on error
			}
			if !isSymlink {
				if _, statErr := os.Stat(childConfigFile); statErr == nil {
					// 파일 존재 여부만 확인 - auto-generated/user-maintained 구분 없이 경고 후 교체
					fmt.Fprintf(out,
						"  ⚠ workspace '%s': replacing existing config with configLink symlink: %s\n",
						name, childConfigFile)
				}
			}
			fmt.Fprintf(out, "→ Linking workspace '%s': %s → %s\n", name, childConfigFile, ws.ConfigLink)
			if err := config.CreateConfigSymlinkForce(ws.ConfigLink, wsPath, configDir); err != nil {
				return fmt.Errorf("failed to create config symlink for workspace '%s': %w", name, err)
			}
			continue
		}

		// Check if .gz-git.yaml exists
		childConfigFile := filepath.Join(wsPath, ".gz-git.yaml")
		if _, err := os.Stat(childConfigFile); os.IsNotExist(err) {
			// Missing! Create it.
			fmt.Fprintf(out, "→ Bootstrapping workspace '%s': creating %s\n", name, childConfigFile)

			if err := os.MkdirAll(wsPath, 0o750); err != nil {
				return fmt.Errorf("failed to create directory for workspace '%s': %w", name, err)
			}

			// Generate bootstrap config using template
			content, err := templates.Render(templates.RepositoriesBootstrap, templates.BootstrapData{
				Parent:  cfg.ConfigPath,
				Profile: ws.Profile,
			})
			if err != nil {
				return fmt.Errorf("failed to render bootstrap template for '%s': %w", name, err)
			}

			if err := os.WriteFile(childConfigFile, []byte(content), 0o600); err != nil {
				return fmt.Errorf("failed to write bootstrap config for '%s': %w", name, err)
			}
		}
	}
	return nil
}

// deduplicateActions removes duplicate actions targeting the same path.
// When multiple planners produce actions for the same repo, the first one wins.
func deduplicateActions(actions []reposync.Action) []reposync.Action {
	seen := make(map[string]bool, len(actions))
	result := make([]reposync.Action, 0, len(actions))
	for _, a := range actions {
		key := a.Repo.TargetPath
		if key == "" {
			key = a.Repo.Name
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, a)
	}
	return result
}

// resolveWorkspacePath resolves a workspace path that may be:
// - Absolute: /path/to/workspace
// - Home-relative: ~/path/to/workspace
// - Relative: ./path/to/workspace or path/to/workspace
//
// Relative paths are resolved from baseDir.
func resolveWorkspacePath(path, baseDir string) string {
	if path == "" {
		return baseDir
	}

	// Handle home-relative paths
	if len(path) > 1 && path[0] == '~' && path[1] == '/' {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}

	// Handle absolute paths
	if filepath.IsAbs(path) {
		return path
	}

	// Handle relative paths
	return filepath.Join(baseDir, path)
}

// executeSelfSync syncs the config directory itself if selfSync is enabled.
// This runs before workspace sync to ensure the devbox is up-to-date.
func executeSelfSync(ctx context.Context, cfg *config.Config, configDir string, out io.Writer, dryRun bool) error {
	if cfg.SelfSync == nil || !cfg.SelfSync.Enabled {
		return nil
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "=== Self-sync (config directory) ===")

	// Check if configDir is a git repository
	gitDir := filepath.Join(configDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		fmt.Fprintf(out, "⚠️  Skipping self-sync: %s is not a git repository\n", configDir)
		return nil
	}

	// Determine strategy (default to fetch for safety)
	strategy := cfg.SelfSync.Strategy
	if strategy == "" {
		strategy = "fetch"
	}

	// Disallow reset for self-sync
	if strategy == "reset" {
		fmt.Fprintf(out, "⚠️  Strategy 'reset' not allowed for self-sync, falling back to 'fetch'\n")
		strategy = "fetch"
	}

	if dryRun {
		fmt.Fprintf(out, "  [dry-run] Would %s %s\n", strategy, configDir)
		return nil
	}

	// Check if working tree is dirty
	isDirty, err := isWorkingTreeDirty(ctx, configDir)
	if err != nil {
		fmt.Fprintf(out, "⚠️  Failed to check working tree status: %v\n", err)
		// Continue with fetch as safe fallback
		strategy = "fetch"
	}

	if isDirty && strategy == "pull" {
		fmt.Fprintf(out, "  Working tree is dirty, falling back to fetch\n")
		strategy = "fetch"
	}

	// Execute the sync
	switch strategy {
	case "fetch":
		fmt.Fprintf(out, "  Fetching %s...\n", configDir)
		if err := gitFetch(ctx, configDir); err != nil {
			return fmt.Errorf("self-sync fetch failed: %w", err)
		}
		fmt.Fprintf(out, "  ✓ Fetched successfully\n")
	case "pull":
		fmt.Fprintf(out, "  Pulling %s...\n", configDir)
		if err := gitPull(ctx, configDir); err != nil {
			return fmt.Errorf("self-sync pull failed: %w", err)
		}
		fmt.Fprintf(out, "  ✓ Pulled successfully\n")
	case "skip":
		fmt.Fprintf(out, "  Skipping (strategy=skip)\n")
	default:
		fmt.Fprintf(out, "⚠️  Unknown self-sync strategy '%s', skipping\n", strategy)
	}

	fmt.Fprintln(out, "")
	return nil
}

// isWorkingTreeDirty checks if the git working tree has uncommitted changes.
//
// Plain --porcelain is enough here, unlike the call sites that read individual
// records: the answer is whether git reported anything at all, so neither the
// C-quoting of unusual paths nor the collapsing of untracked directories into a
// single entry can change it. cmd.Output() also reports a non-zero exit as an
// error, so an unreadable repository is not silently answered "clean" — every
// caller must decide what to do with that error rather than inherit a false.
func isWorkingTreeDirty(ctx context.Context, repoPath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain") // #nosec G204 -- fixed git status query in the selected repository.
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(output)) != "", nil
}

// gitFetch runs git fetch --all in the specified directory.
func gitFetch(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "--all", "--prune") // #nosec G204 -- fixed git fetch query in the selected repository.
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// gitPull runs git pull in the specified directory.
func gitPull(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "pull") // #nosec G204 -- fixed git pull query in the selected repository.
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// syncSummary holds counts of different action types for preview.
type syncSummary struct {
	Clone   int
	Update  int
	Skip    int
	Delete  int
	Total   int
	Actions []reposync.Action
}

// RepoChanges holds detailed change information for a single repository.
type RepoChanges struct {
	RepoName  string
	Action    reposync.ActionType
	Path      string
	URL       string
	Files     FileChangeSummary
	Conflicts []ConflictInfo
	Warnings  []string
	Diverged  bool // Tracks if local branch has diverged from remote
}

// FileChangeSummary holds file-level change statistics.
type FileChangeSummary struct {
	Added    []string
	Modified []string
	Deleted  []string
	Total    int
}

// ConflictInfo describes a detected conflict.
type ConflictInfo struct {
	FilePath      string
	ConflictType  string // "local-changes", "diverged-branches", "dirty-worktree"
	LocalChanges  bool   // Has uncommitted local changes
	RemoteChanges bool   // Has incoming remote changes
	Description   string // Human-readable description
}

// ConflictType constants.
const (
	ConflictTypeLocalChanges     = "local-changes"
	ConflictTypeDivergedBranches = "diverged-branches"
	ConflictTypeDirtyWorktree    = "dirty-worktree"
)

// buildSyncSummary analyzes actions and counts by type.
func buildSyncSummary(actions []reposync.Action) syncSummary {
	summary := syncSummary{
		Total:   len(actions),
		Actions: actions,
	}
	for _, action := range actions {
		switch action.Type {
		case reposync.ActionClone:
			summary.Clone++
		case reposync.ActionUpdate:
			summary.Update++
		case reposync.ActionSkip:
			summary.Skip++
		case reposync.ActionDelete:
			summary.Delete++
		}
	}
	return summary
}

// analyzeRepoChanges performs detailed analysis of changes for a repository.
// Returns nil if repository doesn't exist yet (new clone) or analysis fails.
func analyzeRepoChanges(ctx context.Context, action reposync.Action) (*RepoChanges, error) { //nolint:unparam // error is always nil; kept for consistent error-return interface
	changes := &RepoChanges{
		RepoName: action.Repo.Name,
		Action:   action.Type,
		Path:     action.Repo.TargetPath,
		URL:      action.Repo.CloneURL,
		Warnings: []string{},
	}

	// Skip analysis for new clones (directory doesn't exist yet)
	if action.Type == reposync.ActionClone {
		return changes, nil
	}

	// Skip analysis if path doesn't exist
	if _, err := os.Stat(action.Repo.TargetPath); os.IsNotExist(err) {
		return changes, nil
	}

	// For updates, analyze file changes and conflicts
	if action.Type == reposync.ActionUpdate {
		// Check working tree status
		isDirty, err := isWorkingTreeDirty(ctx, action.Repo.TargetPath)
		if err != nil {
			changes.Warnings = append(changes.Warnings, fmt.Sprintf("Failed to check working tree: %v", err))
		} else if isDirty {
			changes.Conflicts = append(changes.Conflicts, ConflictInfo{
				ConflictType: ConflictTypeDirtyWorktree,
				LocalChanges: true,
				Description:  "Uncommitted local changes detected",
			})
			changes.Warnings = append(changes.Warnings, "Local uncommitted changes may be overwritten")
		}

		// Get file diff against remote
		baseBranch := action.Repo.Branch
		if baseBranch == "" {
			baseBranch = "HEAD"
		}

		fileDiff, err := getFileDiff(ctx, action.Repo.TargetPath, baseBranch)
		if err != nil {
			changes.Warnings = append(changes.Warnings, fmt.Sprintf("Failed to get diff: %v", err))
		} else {
			changes.Files = fileDiff
		}

		// Check for diverged branches
		diverged, err := checkDivergence(ctx, action.Repo.TargetPath, baseBranch)
		if err != nil {
			changes.Warnings = append(changes.Warnings, fmt.Sprintf("Failed to check divergence: %v", err))
		} else if diverged {
			changes.Diverged = true
			changes.Conflicts = append(changes.Conflicts, ConflictInfo{
				ConflictType:  ConflictTypeDivergedBranches,
				LocalChanges:  true,
				RemoteChanges: true,
				Description:   "Local branch has diverged from remote",
			})
			changes.Warnings = append(changes.Warnings, "Branch has diverged from remote (manual merge may be needed)")
		}
	}

	return changes, nil
}

func validateComparisonBranch(branch string) error {
	if branch == "HEAD" {
		return nil
	}
	if err := gitcmd.SanitizeBranchName(branch); err != nil {
		return fmt.Errorf("invalid comparison branch %q: %w", branch, err)
	}
	return nil
}

// getFileDiff analyzes file changes between local HEAD and remote.
// Returns empty summary if fetch hasn't been done yet.
func getFileDiff(ctx context.Context, repoPath, baseBranch string) (FileChangeSummary, error) { //nolint:unparam // error is always nil; kept for consistent error-return interface
	summary := FileChangeSummary{
		Added:    []string{},
		Modified: []string{},
		Deleted:  []string{},
	}
	if err := validateComparisonBranch(baseBranch); err != nil {
		return summary, err
	}

	// Construct remote tracking branch name.
	// When branch is explicitly set, use origin/{branch}.
	// When branch is "HEAD" (default), use origin/HEAD which always exists
	// as a symbolic ref to the remote default branch. Avoid @{u} because it
	// requires an explicit tracking configuration and fails on detached HEADs.
	remoteBranch := fmt.Sprintf("origin/%s", baseBranch)
	if baseBranch == "HEAD" {
		remoteBranch = "origin/HEAD"
	}

	// Run git diff --name-status HEAD..origin/branch
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-status", fmt.Sprintf("HEAD..%s", remoteBranch)) // #nosec G204 -- baseBranch is validated and the command has no shell.
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// If remote branch doesn't exist or fetch not done, return empty summary.
		return summary, nil //nolint:nilerr // git errors here (no remote/fetch) are expected; caller handles empty summary
	}

	// Parse output: each line is "STATUS\tFILENAME"
	lines := strings.SplitSeq(strings.TrimSpace(string(output)), "\n")
	for line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status := parts[0]
		filename := parts[1]

		switch status[0] {
		case 'A':
			summary.Added = append(summary.Added, filename)
		case 'M':
			summary.Modified = append(summary.Modified, filename)
		case 'D':
			summary.Deleted = append(summary.Deleted, filename)
		}
	}

	summary.Total = len(summary.Added) + len(summary.Modified) + len(summary.Deleted)
	return summary, nil
}

// checkDivergence checks if local branch has diverged from remote.
// Returns true if local has commits not in remote AND remote has commits not in local.
func checkDivergence(ctx context.Context, repoPath, baseBranch string) (bool, error) { //nolint:unparam // error is always nil; kept for consistent error-return interface
	if err := validateComparisonBranch(baseBranch); err != nil {
		return false, err
	}
	remoteBranch := fmt.Sprintf("origin/%s", baseBranch)
	if baseBranch == "HEAD" {
		remoteBranch = "origin/HEAD"
	}

	// Check commits in local not in remote
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", fmt.Sprintf("%s..HEAD", remoteBranch)) // #nosec G204 -- baseBranch is validated and the command has no shell.
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return false, nil //nolint:nilerr // remote branch may not exist yet; treat as not diverged
	}
	localAhead := strings.TrimSpace(string(output))

	// Check commits in remote not in local
	cmd = exec.CommandContext(ctx, "git", "rev-list", "--count", fmt.Sprintf("HEAD..%s", remoteBranch)) // #nosec G204 -- baseBranch is validated and the command has no shell.
	cmd.Dir = repoPath
	output, err = cmd.Output()
	if err != nil {
		return false, nil //nolint:nilerr // remote branch may not exist yet; treat as not diverged
	}
	remoteBehind := strings.TrimSpace(string(output))

	// Diverged if both have commits.
	// git rev-list --count always outputs a valid decimal integer; parse errors default to not-diverged.
	localCount, err := strconv.Atoi(localAhead)
	if err != nil {
		return false, nil //nolint:nilerr // parse failure means count unknown; treat as not diverged
	}
	remoteCount, err := strconv.Atoi(remoteBehind)
	if err != nil {
		return false, nil //nolint:nilerr // parse failure means count unknown; treat as not diverged
	}

	return localCount > 0 && remoteCount > 0, nil
}

// displaySyncSummary prints a preview summary to the output.
func displaySyncSummary(out io.Writer, s syncSummary) {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "═══ Sync Preview ═══")
	fmt.Fprintf(out, "Total: %d repositories\n", s.Total)
	fmt.Fprintln(out, "")

	if s.Clone > 0 {
		fmt.Fprintf(out, "  + %d will be cloned (new)\n", s.Clone)
	}
	if s.Update > 0 {
		fmt.Fprintf(out, "  ↓ %d will be updated\n", s.Update)
	}
	if s.Skip > 0 {
		fmt.Fprintf(out, "  ⊘ %d will be skipped\n", s.Skip)
	}
	if s.Delete > 0 {
		fmt.Fprintf(out, "  ✗ %d will be deleted\n", s.Delete)
	}
	fmt.Fprintln(out, "")
}

// displayCompactSyncPreview shows a lightweight one-line summary before sync.
// Used by default (without --verbose). Only highlights repos with warnings.
func displayCompactSyncPreview(ctx context.Context, out io.Writer, s syncSummary, actions []reposync.Action) {
	// Brief counts line
	var parts []string
	if s.Clone > 0 {
		parts = append(parts, fmt.Sprintf("+%d clone", s.Clone))
	}
	if s.Update > 0 {
		parts = append(parts, fmt.Sprintf("↓%d update", s.Update))
	}
	if s.Skip > 0 {
		parts = append(parts, fmt.Sprintf("⊘%d skip", s.Skip))
	}
	if s.Delete > 0 {
		parts = append(parts, fmt.Sprintf("✗%d delete", s.Delete))
	}
	summary := strings.Join(parts, "  ")
	fmt.Fprintf(out, "\nSyncing %d repositories  [%s]\n", s.Total, summary)

	// Group actions by workspace for display
	type wsGroup struct {
		name    string
		actions []reposync.Action
	}
	var groups []wsGroup
	for _, action := range actions {
		ws := action.Workspace
		if len(groups) == 0 || groups[len(groups)-1].name != ws {
			groups = append(groups, wsGroup{name: ws})
		}
		groups[len(groups)-1].actions = append(groups[len(groups)-1].actions, action)
	}

	// Show workspace-grouped warnings
	hasMultipleGroups := len(groups) > 1 || (len(groups) == 1 && groups[0].name != "")
	for _, g := range groups {
		if hasMultipleGroups {
			name := g.name
			if name == "" {
				name = "(repositories)"
			}
			fmt.Fprintf(out, "\n  ── %s (%d repos) ──\n", name, len(g.actions))
		}

		for _, action := range g.actions {
			if action.Type != reposync.ActionUpdate {
				continue
			}
			path := action.Repo.TargetPath
			if path == "" {
				continue
			}
			// This block is the preview's whole account of what a sync may
			// overwrite, so a repository that could not be checked has to say so.
			// Staying silent puts it among the repositories that were checked and
			// found safe, which is the reading that leads to pressing enter.
			isDirty, err := isWorkingTreeDirty(ctx, path)

			switch {
			case err != nil:
				fmt.Fprintf(out, "  ⚠  %-28s  could not be checked for local changes: %v\n", action.Repo.Name, err)
			case isDirty:
				fmt.Fprintf(out, "  ⚠  %-28s  has local changes (may be overwritten)\n", action.Repo.Name)
			}
		}
	}
}

// buildDetailedSyncPreview analyzes all actions and collects detailed change information.
func buildDetailedSyncPreview(ctx context.Context, actions []reposync.Action) ([]RepoChanges, syncSummary) {
	var detailedChanges []RepoChanges
	summary := buildSyncSummary(actions)

	for _, action := range actions {
		changes, err := analyzeRepoChanges(ctx, action)
		if err != nil {
			// Log error but continue with other repos
			continue
		}
		if changes != nil {
			detailedChanges = append(detailedChanges, *changes)
		}
	}

	return detailedChanges, summary
}

// displayDetailedSyncPreview shows comprehensive preview including file-level changes and conflicts.
func displayDetailedSyncPreview(out io.Writer, changes []RepoChanges, summary syncSummary) {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "═══════════════════════════════════════════════")
	fmt.Fprintln(out, "           Sync Preview (Detailed)            ")
	fmt.Fprintln(out, "═══════════════════════════════════════════════")
	fmt.Fprintf(out, "Total: %d repositories\n", summary.Total)
	fmt.Fprintln(out, "")

	// Show high-level summary
	if summary.Clone > 0 {
		fmt.Fprintf(out, "  + %d will be cloned (new)\n", summary.Clone)
	}
	if summary.Update > 0 {
		fmt.Fprintf(out, "  ↓ %d will be updated\n", summary.Update)
	}
	if summary.Skip > 0 {
		fmt.Fprintf(out, "  ⊘ %d will be skipped\n", summary.Skip)
	}
	if summary.Delete > 0 {
		fmt.Fprintf(out, "  ✗ %d will be deleted\n", summary.Delete)
	}
	fmt.Fprintln(out, "")

	// Collect warnings
	var allWarnings []string
	for _, change := range changes {
		if len(change.Warnings) > 0 {
			for _, warning := range change.Warnings {
				allWarnings = append(allWarnings, fmt.Sprintf("%s: %s", change.RepoName, warning))
			}
		}
	}

	// Show warnings first if any
	if len(allWarnings) > 0 {
		fmt.Fprintln(out, "⚠️  Warnings:")
		for _, warning := range allWarnings {
			fmt.Fprintf(out, "  • %s\n", warning)
		}
		fmt.Fprintln(out, "")
	}

	// Show detailed repository changes.
	// Skip repos are omitted from the detail section to keep output concise;
	// they contribute to the summary count above.
	fmt.Fprintln(out, "Repository Details:")
	fmt.Fprintln(out, "")

	for _, change := range changes {
		if change.Action == reposync.ActionSkip {
			continue
		}
		displayRepoChange(out, change)
	}

	fmt.Fprintln(out, "═══════════════════════════════════════════════")
}

// displayRepoChange shows detailed information for a single repository.
func displayRepoChange(out io.Writer, change RepoChanges) {
	// Action symbol
	var actionSymbol string
	var actionDesc string
	switch change.Action {
	case reposync.ActionClone:
		actionSymbol = "+"
		actionDesc = "clone"
	case reposync.ActionUpdate:
		actionSymbol = "↓"
		actionDesc = "update"
	case reposync.ActionSkip:
		actionSymbol = "⊘"
		actionDesc = "skip - up to date"
	case reposync.ActionDelete:
		actionSymbol = "✗"
		actionDesc = "delete"
	default:
		actionSymbol = "?"
		actionDesc = "unknown"
	}

	// Show repo header
	fmt.Fprintf(out, "  %s %s (%s)\n", actionSymbol, change.RepoName, actionDesc)

	// Show URL for clones
	if change.Action == reposync.ActionClone && change.URL != "" {
		fmt.Fprintf(out, "    → %s\n", change.URL)
	}

	// Show conflicts
	if len(change.Conflicts) > 0 {
		for _, conflict := range change.Conflicts {
			fmt.Fprintf(out, "    ⚠️  %s\n", conflict.Description)
		}
	}

	// Show file changes for updates
	if change.Action == reposync.ActionUpdate && change.Files.Total > 0 {
		fmt.Fprintf(out, "    Files: ")
		var parts []string
		if len(change.Files.Added) > 0 {
			parts = append(parts, fmt.Sprintf("+%d added", len(change.Files.Added)))
		}
		if len(change.Files.Modified) > 0 {
			parts = append(parts, fmt.Sprintf("~%d modified", len(change.Files.Modified)))
		}
		if len(change.Files.Deleted) > 0 {
			parts = append(parts, fmt.Sprintf("-%d deleted", len(change.Files.Deleted)))
		}
		fmt.Fprintf(out, "%s\n", strings.Join(parts, ", "))

		// Show file names (limit to first 5 of each type)
		if len(change.Files.Modified) > 0 {
			displayFileList(out, "modified", change.Files.Modified, 5)
		}
		if len(change.Files.Added) > 0 {
			displayFileList(out, "added", change.Files.Added, 5)
		}
		if len(change.Files.Deleted) > 0 {
			displayFileList(out, "deleted", change.Files.Deleted, 5)
		}
	}

	// Show divergence warning
	if change.Diverged {
		fmt.Fprintln(out, "    ⚠️  Branch has diverged from remote")
	}

	fmt.Fprintln(out, "")
}

// displayFileList shows a list of files with optional truncation.
func displayFileList(out io.Writer, label string, files []string, maxShow int) {
	if len(files) == 0 {
		return
	}

	shown := files
	truncated := false
	if len(files) > maxShow {
		shown = files[:maxShow]
		truncated = true
	}

	for _, file := range shown {
		fmt.Fprintf(out, "      → %s\n", file)
	}

	if truncated {
		fmt.Fprintf(out, "      ... and %d more %s\n", len(files)-maxShow, label)
	}
}

// confirmSyncPrompt displays an interactive confirmation prompt.
// Returns true if user wants to proceed, false otherwise.
func confirmSyncPrompt() (bool, error) {
	var proceed bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Proceed with sync?").
				Affirmative("Yes").
				Negative("No").
				Value(&proceed),
		),
	)

	err := form.Run()
	if err != nil {
		return false, err
	}
	return proceed, nil
}

// isTerminal checks if stdout is a terminal (not redirected/piped).
// Returns false in CI/non-interactive environments.
func isTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// ─── Recursive child workspace sync ───────────────────────────────────────────

// recursiveSyncOpts holds settings for recursive child workspace sync.
type recursiveSyncOpts struct {
	MaxDepth int
	Visited  map[string]bool // absolute paths already processed
	Out      io.Writer
	Depth    int
	Strategy string
	Parallel int
	DryRun   bool
}

// syncChildWorkspaces scans succeeded execution results for child workspaces
// containing .gz-git.yaml and recursively syncs them.
func syncChildWorkspaces(ctx context.Context, result reposync.ExecutionResult, opts recursiveSyncOpts) {
	if opts.Depth >= opts.MaxDepth {
		fmt.Fprintf(opts.Out, "\n[recursive] Max depth %d reached, stopping.\n", opts.MaxDepth)
		return
	}

	var childDirs []string
	for _, r := range result.Succeeded {
		targetPath := r.Action.Repo.TargetPath
		if targetPath == "" {
			continue
		}

		absPath, err := filepath.Abs(targetPath)
		if err != nil {
			continue
		}

		if opts.Visited[absPath] {
			continue
		}

		configPath := filepath.Join(absPath, DefaultConfigFile)
		if _, err := os.Stat(configPath); err == nil {
			childDirs = append(childDirs, absPath)
		}
	}

	if len(childDirs) == 0 {
		return
	}

	fmt.Fprintf(opts.Out, "\n[recursive] Found %d child workspace(s) at depth %d:\n", len(childDirs), opts.Depth+1)
	for _, dir := range childDirs {
		fmt.Fprintf(opts.Out, "  → %s\n", dir)
	}

	for _, dir := range childDirs {
		runChildSync(ctx, dir, opts)
	}
}

// runChildSync loads config, plans, and executes sync for a single child workspace.
// Errors are reported as warnings and do not propagate to the parent.
func runChildSync(ctx context.Context, childDir string, opts recursiveSyncOpts) {
	absPath, err := filepath.Abs(childDir)
	if err != nil {
		absPath = childDir
	}
	opts.Visited[absPath] = true

	configPath := filepath.Join(childDir, DefaultConfigFile)
	fmt.Fprintf(opts.Out, "\n[recursive] Syncing child workspace: %s (depth %d)\n", childDir, opts.Depth+1)

	// Load flat config
	loader := FileSpecLoader{}
	cfgData, err := loader.Load(ctx, configPath)
	if err != nil {
		fmt.Fprintf(opts.Out, "  ⚠️  Failed to load config: %v\n", err)
		return
	}

	configFile := filepath.Base(configPath)

	// Plan from flat config
	var allActions []reposync.Action
	if len(cfgData.Plan.Input.Repos) > 0 {
		flatActions := createActionsFromFlatConfig(cfgData.Plan, childDir)
		allActions = append(allActions, flatActions...)
	}

	// Load recursive/hierarchical config
	recursiveCfg, recursiveErr := config.LoadConfigRecursive(childDir, configFile)
	if recursiveErr != nil && !os.IsNotExist(recursiveErr) {
		fmt.Fprintf(opts.Out, "  ⚠️  Config warning: %v\n", recursiveErr)
	}

	if recursiveCfg != nil {
		if err := config.LoadWorkspaces(childDir, recursiveCfg, config.HybridMode); err == nil {
			forgeActions, fErr := planForgeWorkspaces(ctx, recursiveCfg, opts.Out, opts.Strategy, false)
			if fErr != nil {
				fmt.Fprintf(opts.Out, "  ⚠️  Failed to plan forge workspaces: %v\n", fErr)
			} else {
				allActions = append(allActions, forgeActions...)
			}

			gitActions, gErr := planGitWorkspaces(ctx, recursiveCfg, childDir, opts.Out, opts.Strategy)
			if gErr != nil {
				fmt.Fprintf(opts.Out, "  ⚠️  Failed to plan git workspaces: %v\n", gErr)
			} else {
				allActions = append(allActions, gitActions...)
			}
		}
	}

	if len(allActions) == 0 {
		fmt.Fprintf(opts.Out, "  No repositories found in child workspace.\n")
		return
	}

	fmt.Fprintf(opts.Out, "  → %d repositories to sync\n", len(allActions))

	// Execute sync
	staticPlanner := &precomputedPlanner{actions: allActions}
	executor := reposync.GitExecutor{}
	state := reposync.NewInMemoryStateStore()
	orch := reposync.NewOrchestrator(staticPlanner, executor, state)

	runOpts := cfgData.Run
	if opts.Parallel > 0 {
		runOpts.Parallel = opts.Parallel
	}

	// Use consoleProgress (no ANSI in-place) for child syncs to avoid display conflicts
	progress := &consoleProgress{out: opts.Out}

	execResult, err := orch.Run(ctx, reposync.RunRequest{
		RunOptions: runOpts,
		Progress:   progress,
		State:      state,
	})
	if err != nil {
		fmt.Fprintf(opts.Out, "  ⚠️  Child workspace sync failed: %v\n", err)
		return
	}

	succeeded := len(execResult.Succeeded)
	failed := len(execResult.Failed)
	fmt.Fprintf(opts.Out, "  [recursive] Done: %d succeeded, %d failed\n", succeeded, failed)

	// Continue recursion into deeper children
	childOpts := opts
	childOpts.Depth = opts.Depth + 1
	syncChildWorkspaces(ctx, execResult, childOpts)
}

// errSyncPartialFailure returns a non-nil error when some repositories failed
// to sync, so the process exit code reflects partial failure in scripts and CI.
func errSyncPartialFailure(result reposync.ExecutionResult) error {
	failed := len(result.Failed)
	if failed == 0 {
		return nil
	}
	total := failed + len(result.Succeeded) + len(result.Skipped)
	return fmt.Errorf("%d of %d repositories failed to sync", failed, total)
}

// buildResultTree constructs a TreeNode hierarchy from execution results.
// It groups succeeded repos by workspace and adds compact status as the message.
func buildResultTree(result reposync.ExecutionResult, rootName string) *TreeNode {
	root := &TreeNode{Name: rootName}

	// Group by workspace
	type wsGroup struct {
		name  string
		repos []reposync.ActionResult
	}
	var groups []wsGroup
	groupIdx := map[string]int{}

	allResults := make([]reposync.ActionResult, 0, len(result.Succeeded)+len(result.Failed))
	allResults = append(allResults, result.Succeeded...)
	allResults = append(allResults, result.Failed...)
	for _, r := range allResults {
		ws := r.Action.Workspace
		if idx, ok := groupIdx[ws]; ok {
			groups[idx].repos = append(groups[idx].repos, r)
		} else {
			groupIdx[ws] = len(groups)
			groups = append(groups, wsGroup{name: ws, repos: []reposync.ActionResult{r}})
		}
	}

	for _, g := range groups {
		if g.name == "" {
			// Flat repos (no workspace) → add directly as children
			for _, r := range g.repos {
				status := "synced"
				if r.Error != nil {
					status = "failed"
				}
				msg := reposync.FormatCompactStatus(r.PostStatus)
				root.Children = append(root.Children, &TreeNode{
					Name:    r.Action.Repo.Name,
					Status:  status,
					Message: msg,
				})
			}
		} else {
			// Workspace group → create parent node with children
			wsNode := &TreeNode{
				Name:    g.name,
				Message: fmt.Sprintf("(%d synced)", countSucceeded(g.repos)),
			}
			for _, r := range g.repos {
				status := "synced"
				if r.Error != nil {
					status = "failed"
				}
				msg := reposync.FormatCompactStatus(r.PostStatus)
				wsNode.Children = append(wsNode.Children, &TreeNode{
					Name:    r.Action.Repo.Name,
					Status:  status,
					Message: msg,
				})
			}
			root.Children = append(root.Children, wsNode)
		}
	}

	return root
}

func countSucceeded(results []reposync.ActionResult) int {
	n := 0
	for _, r := range results {
		if r.Error == nil {
			n++
		}
	}
	return n
}

// scanForChildConfigs shows a dry-run preview of child workspaces that would be
// recursively synced. For existing repos it checks for .gz-git.yaml; for repos
// that will be cloned, it notes them as pending.
func scanForChildConfigs(out io.Writer, actions []reposync.Action, opts recursiveSyncOpts) {
	if opts.Depth+1 >= opts.MaxDepth {
		return
	}

	var existing []string
	var pending []string

	for _, action := range actions {
		targetPath := action.Repo.TargetPath
		if targetPath == "" {
			continue
		}

		absPath, err := filepath.Abs(targetPath)
		if err != nil {
			continue
		}

		if opts.Visited[absPath] {
			continue
		}

		// Check if target directory exists
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			pending = append(pending, targetPath)
			continue
		}

		// Check for child config
		configPath := filepath.Join(absPath, DefaultConfigFile)
		if _, err := os.Stat(configPath); err == nil {
			existing = append(existing, absPath)
		}
	}

	if len(existing) == 0 && len(pending) == 0 {
		return
	}

	fmt.Fprintf(out, "\n[recursive] Child workspace scan (depth %d):\n", opts.Depth+1)
	for _, dir := range existing {
		fmt.Fprintf(out, "  → %s (has %s)\n", dir, DefaultConfigFile)
	}
	for _, dir := range pending {
		fmt.Fprintf(out, "  ? %s (will check after clone)\n", dir)
	}
}

// SyncResultJSON is the JSON representation of a sync operation result.
type SyncResultJSON struct {
	Total     int            `json:"total"`
	Succeeded int            `json:"succeeded"`
	Failed    int            `json:"failed"`
	Duration  int64          `json:"duration_ms"`  //nolint:tagliatelle // existing JSON API field name
	Repos     []SyncRepoJSON `json:"repositories"` //nolint:tagliatelle // existing JSON API field name
}

// SyncRepoJSON represents the JSON output for a single repository in a sync result.
type SyncRepoJSON struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func prepareSyncResultJSON(execResult reposync.ExecutionResult, durationMs int64) SyncResultJSON {
	result := SyncResultJSON{
		Total:     len(execResult.Succeeded) + len(execResult.Failed) + len(execResult.Skipped),
		Succeeded: len(execResult.Succeeded),
		Failed:    len(execResult.Failed),
		Duration:  durationMs,
		Repos:     make([]SyncRepoJSON, 0),
	}

	for _, res := range execResult.Succeeded {
		result.Repos = append(result.Repos, SyncRepoJSON{
			Name:    res.Action.Repo.Name,
			Action:  string(res.Action.Type),
			Status:  "success",
			Message: res.Message,
		})
	}
	for _, res := range execResult.Failed {
		repoJSON := SyncRepoJSON{
			Name:    res.Action.Repo.Name,
			Action:  string(res.Action.Type),
			Status:  "failed",
			Message: res.Message,
		}
		if res.Error != nil {
			repoJSON.Error = res.Error.Error()
		}
		result.Repos = append(result.Repos, repoJSON)
	}
	for _, res := range execResult.Skipped {
		result.Repos = append(result.Repos, SyncRepoJSON{
			Name:    res.Action.Repo.Name,
			Action:  string(res.Action.Type),
			Status:  "skipped",
			Message: res.Message,
		})
	}

	return result
}

func displaySyncResultsJSON(out io.Writer, execResult reposync.ExecutionResult, durationMs int64, verbose bool) error {
	result := prepareSyncResultJSON(execResult, durationMs)
	return cliutil.WriteJSON(out, result, verbose)
}

func displaySyncResultsLLM(out io.Writer, execResult reposync.ExecutionResult, durationMs int64) error {
	result := prepareSyncResultJSON(execResult, durationMs)
	return cliutil.WriteLLM(out, result)
}

// ================================================================================
// Post-sync push support
// ================================================================================

type pushResult struct {
	Path    string
	Pushed  bool
	Commits int
	Message string
	Error   error
}

type pushTarget struct {
	Path     string
	ReadOnly bool
}

// collectPushTargets returns target paths from successfully synced repos.
func collectPushTargets(result reposync.ExecutionResult) []pushTarget {
	targets := make([]pushTarget, 0, len(result.Succeeded))
	for _, r := range result.Succeeded {
		targets = append(targets, pushTarget{Path: r.Action.Repo.TargetPath, ReadOnly: r.Action.ReadOnly})
	}
	return targets
}

// executePushForSyncedRepos pushes repos that have local commits ahead of remote.
func executePushForSyncedRepos(ctx context.Context, targets []pushTarget, parallel int) []pushResult {
	if parallel <= 0 {
		parallel = repository.DefaultForgeParallel
	}

	results := make([]pushResult, len(targets))

	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup

	for i, target := range targets {
		wg.Add(1)
		go func(idx int, repo pushTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = pushOneRepo(ctx, repo)
		}(i, target)
	}

	wg.Wait()
	return results
}

func pushOneRepo(ctx context.Context, target pushTarget) pushResult {
	repoPath := target.Path
	if target.ReadOnly {
		return pushResult{Path: repoPath, Message: "read-only workspace; push skipped"}
	}
	access, source, err := config.WorkspacePushAccessContext(ctx, repoPath)
	if err != nil {
		return pushResult{Path: repoPath, Error: fmt.Errorf("resolve workspace access: %w", err)}
	}
	if access.IsReadOnly() {
		return pushResult{Path: repoPath, Message: fmt.Sprintf("read-only workspace from %s; push skipped", source)}
	}
	// Get current branch
	branchCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD") // #nosec G204 -- fixed git query in the selected repository.
	branchOut, err := branchCmd.Output()
	if err != nil {
		return pushResult{Path: repoPath, Error: fmt.Errorf("failed to get current branch: %w", err)}
	}
	branch := strings.TrimSpace(string(branchOut))
	if branch == "HEAD" {
		return pushResult{Path: repoPath, Pushed: false, Message: "detached HEAD; skipping push"}
	}
	if err := gitcmd.SanitizeBranchName(branch); err != nil {
		return pushResult{Path: repoPath, Error: fmt.Errorf("invalid current branch %q: %w", branch, err)}
	}

	// Count commits ahead of remote
	countCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-list", "--count", fmt.Sprintf("origin/%s..HEAD", branch)) // #nosec G204 -- branch is validated and the command has no shell.
	countOut, err := countCmd.Output()
	if err != nil {
		// Remote tracking branch may not exist (e.g. freshly cloned)
		return pushResult{Path: repoPath, Pushed: false, Message: "no remote tracking branch"}
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(countOut)))
	if err != nil {
		count = 0 // treat parse failure as nothing to push
	}
	if count == 0 {
		return pushResult{Path: repoPath, Pushed: false, Message: "nothing to push"}
	}

	// Push
	pushCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "push", "origin", branch) // #nosec G204 -- branch is validated and passed as a separate argv value.
	pushOutput, err := pushCmd.CombinedOutput()
	if err != nil {
		return pushResult{
			Path:    repoPath,
			Commits: count,
			Error:   fmt.Errorf("push failed: %s", strings.TrimSpace(string(pushOutput))),
		}
	}

	return pushResult{
		Path:    repoPath,
		Pushed:  true,
		Commits: count,
		Message: fmt.Sprintf("%d commit(s) pushed", count),
	}
}

func displayPushResults(out io.Writer, results []pushResult, isMachine bool) {
	if isMachine {
		return
	}

	for _, r := range results {
		name := filepath.Base(r.Path)
		switch {
		case r.Error != nil:
			fmt.Fprintf(out, "  ✗ %s (%v)\n", name, r.Error)
		case r.Pushed:
			fmt.Fprintf(out, "  ✓ %s (%s)\n", name, r.Message)
		default:
			fmt.Fprintf(out, "  - %s (%s)\n", name, r.Message)
		}
	}
}
