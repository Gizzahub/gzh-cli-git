package reposync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-gitforge/internal/porcelain"
	repo "github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// GitExecutor executes plans using gzh-cli-gitforge for actual Git operations.
type GitExecutor struct {
	Client repo.Client
	Logger repo.Logger
}

// Execute runs the plan with concurrency, retries, and optional dry-run.
func (e GitExecutor) Execute(ctx context.Context, plan Plan, opts RunOptions, sink ProgressSink, store StateStore) (ExecutionResult, error) {
	if sink == nil {
		sink = NoopProgressSink{}
	}
	if store == nil {
		store = NewInMemoryStateStore()
	}

	client := e.Client
	if client == nil {
		client = repo.NewClient(repo.WithClientLogger(e.Logger))
	}
	logger := e.Logger
	if logger == nil {
		logger = nopGitLogger{}
	}

	parallel := opts.Parallel
	if parallel <= 0 {
		parallel = repo.DefaultForgeParallel
	}

	type jobResult struct {
		result ActionResult
		err    error
	}

	jobs := make(chan Action)
	results := make(chan jobResult)

	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for action := range jobs {
			res, err := e.executeOne(ctx, client, logger, action, opts, sink)
			results <- jobResult{result: res, err: err}
		}
	}

	wg.Add(parallel)
	for i := 0; i < parallel; i++ {
		go worker()
	}

	go func() {
		defer close(jobs)
		for _, action := range plan.Actions {
			if ctx.Err() != nil {
				return
			}
			jobs <- action
		}
	}()

	var execResult ExecutionResult
	var state RunState

	done := make(chan struct{})
	go func() {
		defer close(done)
		for r := range results {
			res := r.result
			if r.err != nil {
				res.Error = r.err
			}

			switch {
			case res.Error != nil:
				execResult.Failed = append(execResult.Failed, res)
				state.Items = append(state.Items, RunStateItem{
					Repo:    res.Action.Repo,
					Status:  RunStatusFailed,
					Message: res.Message,
				})
			case res.Action.Type == ActionSkip:
				execResult.Skipped = append(execResult.Skipped, res)
				state.Items = append(state.Items, RunStateItem{
					Repo:    res.Action.Repo,
					Status:  RunStatusDone,
					Message: res.Message,
				})
			default:
				execResult.Succeeded = append(execResult.Succeeded, res)
				state.Items = append(state.Items, RunStateItem{
					Repo:    res.Action.Repo,
					Status:  RunStatusDone,
					Message: res.Message,
				})
			}
		}
	}()

	wg.Wait()
	close(results)
	<-done

	if err := store.Save(ctx, state); err != nil {
		return execResult, fmt.Errorf("save state: %w", err)
	}

	return execResult, nil
}

func (e GitExecutor) executeOne(ctx context.Context, client repo.Client, logger repo.Logger, action Action, opts RunOptions, sink ProgressSink) (ActionResult, error) {
	sink.OnStart(action)

	if opts.DryRun {
		msg := fmt.Sprintf("dry-run: would %s %s", action.Type, action.Repo.TargetPath)
		sink.OnProgress(action, msg, 1.0)
		res := ActionResult{
			Action:  action,
			Message: msg,
		}
		sink.OnComplete(res)
		return res, nil
	}

	switch action.Type {
	case ActionClone, ActionUpdate:
		if action.Repo.CloneURL == "" || action.Repo.TargetPath == "" {
			err := errors.New("missing clone url or target path")
			res := ActionResult{
				Action:  action,
				Message: "invalid action parameters",
				Error:   err,
			}
			sink.OnComplete(res)
			return res, err
		}
		if err := ensureParentDir(action.Repo.TargetPath); err != nil {
			res := ActionResult{
				Action:  action,
				Message: "failed to prepare target directory",
				Error:   err,
			}
			sink.OnComplete(res)
			return res, err
		}
		return e.runCloneOrUpdate(ctx, client, logger, action, opts, sink)
	case ActionDelete:
		if action.Repo.TargetPath == "" {
			err := errors.New("missing target path for delete")
			res := ActionResult{Action: action, Message: "invalid delete target", Error: err}
			sink.OnComplete(res)
			return res, err
		}
		if err := os.RemoveAll(action.Repo.TargetPath); err != nil {
			msg := describeDeleteError(action.Repo.TargetPath, err)
			res := ActionResult{Action: action, Message: msg, Error: err}
			sink.OnComplete(res)
			return res, err
		}
		msg := fmt.Sprintf("deleted %s", action.Repo.TargetPath)
		res := ActionResult{Action: action, Message: msg}
		sink.OnComplete(res)
		return res, nil
	case ActionSkip:
		msg := "skipped"
		res := ActionResult{Action: action, Message: msg}
		sink.OnComplete(res)
		return res, nil
	default:
		err := fmt.Errorf("unsupported action type: %s", action.Type)
		res := ActionResult{Action: action, Message: "unsupported action", Error: err}
		sink.OnComplete(res)
		return res, err
	}
}

func ensureParentDir(targetPath string) error {
	dir := filepath.Dir(targetPath)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o750)
}

// describeDeleteError produces a human-readable message when os.RemoveAll fails.
// It walks the directory to detect permission-denied causes and summarizes leftovers.
func describeDeleteError(targetPath string, removeErr error) string {
	if errors.Is(removeErr, os.ErrPermission) {
		return fmt.Sprintf("failed to delete %s: contains files owned by other users, manual removal required (e.g., sudo rm -rf)", targetPath)
	}

	// Walk to find the first permission-denied file for better diagnostics
	var permDeniedPath string
	walkErr := filepath.WalkDir(targetPath, func(path string, _ os.DirEntry, err error) error {
		if err != nil && errors.Is(err, os.ErrPermission) {
			permDeniedPath = path
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil && permDeniedPath == "" {
		return fmt.Sprintf("failed to delete %s: unable to inspect leftovers: %v", targetPath, walkErr)
	}

	if permDeniedPath != "" {
		return fmt.Sprintf("failed to delete %s: contains files owned by other users, manual removal required (e.g., sudo rm -rf); permission denied at: %s", targetPath, permDeniedPath)
	}

	return fmt.Sprintf("failed to delete %s: %v", targetPath, removeErr)
}

func (e GitExecutor) runCloneOrUpdate(ctx context.Context, client repo.Client, logger repo.Logger, action Action, runOpts RunOptions, sink ProgressSink) (ActionResult, error) {
	updateStrategy := toUpdateStrategy(action)

	progress := &progressAdapter{
		sink:   sink,
		action: action,
	}

	// Prepare authentication (token injection for HTTPS, SSH key for SSH)
	// If auth config is empty, system defaults are used (fallback)
	authResult, err := PrepareAuth(action.Repo.CloneURL, action.Repo.Auth)
	if err != nil {
		res := ActionResult{
			Action:  action,
			Message: fmt.Sprintf("auth setup failed: %v", err),
			Error:   err,
		}
		sink.OnComplete(res)
		return res, err
	}

	// Log warnings (e.g., temporary key creation)
	for _, warning := range authResult.Warnings {
		logger.Warn(warning)
	}

	cleanupAuth := func(operationErr error) error {
		if authResult.TempKeyPath == "" {
			return operationErr
		}
		if cleanupErr := removeTempSSHKey(authResult.TempKeyPath); cleanupErr != nil {
			return errors.Join(operationErr, cleanupErr)
		}
		return operationErr
	}

	attempts := max(runOpts.MaxRetries+1, 1)
	preparation, earlyResult := prepareCloneOrUpdate(ctx, client, logger, action, updateStrategy, authResult, attempts, sink, cleanupAuth)
	if earlyResult != nil {
		sink.OnComplete(*earlyResult)
		return *earlyResult, earlyResult.Error
	}

	cloneOpts := repo.CloneOrUpdateOptions{
		URL:                 authResult.CloneURL,
		Destination:         action.Repo.TargetPath,
		Strategy:            updateStrategy,
		Branch:              preparation.resolvedBranch,
		ExactBranchPrepared: preparation.exactBranchPrepared,
		Logger:              logger,
		Progress:            progress,
		Env:                 authResult.Env,
	}

	result, err := cloneOrUpdateWithRetries(ctx, client, cloneOpts, attempts, sink, action)
	err = cleanupAuth(err)
	if err != nil {
		res := ActionResult{
			Action:  action,
			Message: fmt.Sprintf("clone/update failed after %d attempt(s)", attempts),
			Error:   err,
		}
		sink.OnComplete(res)
		return res, err
	}

	msg := result.Message
	if msg == "" {
		msg = fmt.Sprintf("%s %s", result.Action, action.Repo.TargetPath)
	}

	// Checkout branch if specified
	if preparation.resolvedBranch != "" && !preparation.preUpdateCheckout {
		branchMsg, branchErr := checkoutBranch(ctx, action.Repo.TargetPath, preparation.resolvedBranch, logger)
		if branchErr != nil {
			if action.Repo.StrictBranchCheckout {
				// Strict mode: treat branch checkout failure as action failure
				res := ActionResult{
					Action:  action,
					Message: fmt.Sprintf("%s (branch checkout failed: %v)", msg, branchErr),
					Error:   branchErr,
				}
				sink.OnComplete(res)
				return res, branchErr
			}
			// Lenient mode: warn but continue
			msg = fmt.Sprintf("%s (warning: branch checkout failed: %v)", msg, branchErr)
			logger.Warn(fmt.Sprintf("%s: branch checkout warning: %v", action.Repo.Name, branchErr))
		} else if branchMsg != "" {
			msg = fmt.Sprintf("%s (%s)", msg, branchMsg)
		}
	}

	// Configure additional remotes if specified
	if len(action.Repo.AdditionalRemotes) > 0 {
		remoteMsg, remoteErr := addAdditionalRemotes(ctx, action.Repo.TargetPath, action.Repo.AdditionalRemotes, logger)
		if remoteErr != nil {
			// Non-fatal: warn but continue
			msg = fmt.Sprintf("%s (warning: remote config failed: %v)", msg, remoteErr)
			logger.Warn(fmt.Sprintf("%s: additional remotes warning: %v", action.Repo.Name, remoteErr))
		} else if remoteMsg != "" {
			msg = fmt.Sprintf("%s (%s)", msg, remoteMsg)
		}
	}

	// Collect post-sync status (lightweight: 3 git commands, <100ms total)
	postStatus := collectPostSyncStatus(ctx, action.Repo.TargetPath)

	res := ActionResult{
		Action:     action,
		Message:    msg,
		PostStatus: postStatus,
	}
	sink.OnComplete(res)
	return res, nil
}

type clonePreparation struct {
	resolvedBranch      string
	preUpdateCheckout   bool
	exactBranchPrepared bool
}

func prepareCloneOrUpdate(ctx context.Context, client repo.Client, logger repo.Logger, action Action, strategy repo.UpdateStrategy, auth *AuthResult, attempts int, sink ProgressSink, cleanup func(error) error) (clonePreparation, *ActionResult) {
	targetExists, existingTarget, err := inspectTarget(action.Repo.TargetPath)
	if err != nil {
		err = cleanup(err)
		return clonePreparation{}, &ActionResult{Action: action, Message: fmt.Sprintf("target inspection failed: %v", err), Error: err}
	}
	if targetExists && !existingTarget && strategy != repo.StrategyClone {
		err = cleanup(fmt.Errorf("target directory %q exists but is not a git repository", action.Repo.TargetPath))
		return clonePreparation{}, &ActionResult{Action: action, Message: err.Error(), Error: err}
	}
	if result := dirtyUpdateResult(ctx, client, action, strategy, existingTarget, cleanup); result != nil {
		return clonePreparation{}, result
	}

	probeOrigin := existingTarget && strategy != repo.StrategyClone
	branch, err := resolveConfiguredBranchWithRetries(ctx, action.Repo.TargetPath, auth.CloneURL, auth.Env, action.Repo.Branch, probeOrigin, logger, attempts, sink, action)
	if err != nil {
		err = cleanup(err)
		return clonePreparation{}, &ActionResult{Action: action, Message: fmt.Sprintf("configured branch resolution failed: %v", err), Error: err}
	}
	if !probeOrigin || branch == "" {
		return clonePreparation{resolvedBranch: branch}, nil
	}
	if err = checkoutResolvedBranchBeforeUpdate(ctx, action.Repo.TargetPath, branch, auth.Env, strategy == repo.StrategyReset); err != nil {
		err = cleanup(fmt.Errorf("checkout configured branch %q before update: %w", branch, err))
		return clonePreparation{}, &ActionResult{Action: action, Message: fmt.Sprintf("pre-update branch checkout failed: %v", err), Error: err}
	}
	return clonePreparation{
		resolvedBranch:      branch,
		preUpdateCheckout:   true,
		exactBranchPrepared: strategy == repo.StrategyReset,
	}, nil
}

func dirtyUpdateResult(ctx context.Context, client repo.Client, action Action, strategy repo.UpdateStrategy, existingTarget bool, cleanup func(error) error) *ActionResult {
	if !existingTarget || (strategy != repo.StrategyPull && strategy != repo.StrategyRebase) {
		return nil
	}
	existingRepo, err := client.Open(ctx, action.Repo.TargetPath)
	if err != nil {
		err = cleanup(fmt.Errorf("open existing repository: %w", err))
		return &ActionResult{Action: action, Message: err.Error(), Error: err}
	}
	status, err := client.GetStatus(ctx, existingRepo)
	if err != nil || status.IsClean {
		return nil
	}
	result := &ActionResult{Action: action, Message: "working tree is dirty; update skipped"}
	result.Error = cleanup(nil)
	return result
}

func cloneOrUpdateWithRetries(ctx context.Context, client repo.Client, opts repo.CloneOrUpdateOptions, attempts int, sink ProgressSink, action Action) (*repo.CloneOrUpdateResult, error) {
	var err error
	for i := range attempts {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		result, runErr := client.CloneOrUpdate(ctx, opts)
		if runErr == nil {
			return result, nil
		}
		err = runErr
		if i < attempts-1 {
			sink.OnProgress(action, fmt.Sprintf("retrying (%d/%d): %v", i+1, attempts-1, err), 0)
			time.Sleep(time.Duration(i+1) * 300 * time.Millisecond)
		}
	}
	return nil, err
}

// collectPostSyncStatus runs lightweight git commands to gather branch/ahead/behind/dirty info.
func collectPostSyncStatus(ctx context.Context, repoPath string) *PostSyncStatus {
	ps := &PostSyncStatus{}

	// Branch name
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "branch", "--show-current") // #nosec G204 -- repoPath is the validated repository selected by the sync plan; arguments are fixed.
	if out, err := cmd.Output(); err == nil {
		ps.Branch = strings.TrimSpace(string(out))
	}

	// Ahead/behind upstream
	cmd = exec.CommandContext(ctx, "git", "-C", repoPath, "rev-list", "--left-right", "--count", "HEAD...@{upstream}") // #nosec G204 -- fixed git status query in the selected repository.
	if out, err := cmd.Output(); err == nil {
		parts := strings.Fields(strings.TrimSpace(string(out)))
		if len(parts) == 2 {
			if ahead, err := strconv.Atoi(parts[0]); err == nil {
				ps.AheadBy = ahead
			}
			if behind, err := strconv.Atoi(parts[1]); err == nil {
				ps.BehindBy = behind
			}
		}
	}

	// Porcelain status (dirty + conflicts).
	//
	// This is the one read here whose failure is not safe to leave implicit. A
	// missing branch name or ahead/behind count is visibly absent from the badge,
	// but IsDirty and HasConflicts both default to false, which is exactly what a
	// clean, conflict-free repository looks like. A user who reads "no conflict"
	// off the screen and proceeds with the next sync is acting on a git error.
	cmd = exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain", "-z") // #nosec G204 -- fixed git status query in the selected repository.

	out, err := cmd.Output()
	if err != nil {
		ps.StatusErr = fmt.Errorf("git status failed: %w", err)
		return ps
	}

	records, err := porcelain.Parse(string(out))
	if err != nil {
		ps.StatusErr = fmt.Errorf("git status failed: %w", err)
		return ps
	}

	ps.IsDirty = len(records) > 0

	for _, rec := range records {
		if porcelain.IsUnmerged(rec.Code) {
			ps.HasConflicts = true
			break
		}
	}

	return ps
}

// FormatCompactStatus renders PostSyncStatus as "master|↓5|↑3|dirty".
func FormatCompactStatus(ps *PostSyncStatus) string {
	if ps == nil {
		return ""
	}

	parts := []string{}
	if ps.Branch != "" {
		parts = append(parts, ps.Branch)
	}
	if ps.BehindBy > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", ps.BehindBy))
	}
	if ps.AheadBy > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", ps.AheadBy))
	}
	// An unreadable working tree gets its own token rather than falling through
	// to the clean case, where the absence of "dirty"/"conflict" would read as a
	// positive result the status never established.
	switch {
	case ps.StatusErr != nil:
		parts = append(parts, "unknown")
	case ps.HasConflicts:
		parts = append(parts, "conflict")
	case ps.IsDirty:
		parts = append(parts, "dirty")
	}

	return strings.Join(parts, "|")
}

func toUpdateStrategy(action Action) repo.UpdateStrategy {
	// clone action always forces clone strategy
	if action.Type == ActionClone {
		return repo.StrategyClone
	}

	switch action.Strategy {
	case StrategyPull:
		return repo.StrategyPull
	case StrategyFetch:
		return repo.StrategyFetch
	case StrategyRebase:
		return repo.StrategyRebase
	case StrategyClone:
		return repo.StrategyClone
	case StrategyReset, "":
		return repo.StrategyReset
	default:
		return repo.StrategyReset
	}
}

type progressAdapter struct {
	sink   ProgressSink
	action Action
	total  int64
}

func (p *progressAdapter) Start(total int64) {
	p.total = total
	p.sink.OnProgress(p.action, "start", 0)
}

func (p *progressAdapter) Update(current int64) {
	progress := 0.0
	if p.total > 0 {
		progress = float64(current) / float64(p.total)
	}
	p.sink.OnProgress(p.action, "update", progress)
}

func (p *progressAdapter) Done() {
	p.sink.OnProgress(p.action, "done", 1.0)
}

type nopGitLogger struct{}

func (nopGitLogger) Debug(string, ...any) {}
func (nopGitLogger) Info(string, ...any)  {}
func (nopGitLogger) Warn(string, ...any)  {}
func (nopGitLogger) Error(string, ...any) {}

// resolveConfiguredBranch chooses one remote branch from a comma-separated
// configured fallback list. An empty configuration deliberately returns an
// empty branch so repository.CloneOrUpdate retains its origin/HEAD behavior.
//
// Existing update targets probe origin; clone/reclone targets probe cloneURL
// because CloneOrUpdate will replace the destination before using that URL.
func resolveConfiguredBranch(ctx context.Context, targetPath, cloneURL string, env []string, configured string, probeOrigin bool, logger repo.Logger) (string, error) {
	if configured == "" {
		return "", nil
	}
	if err := gitcmd.SanitizeURL(cloneURL); err != nil {
		return "", fmt.Errorf("invalid clone URL for configured branch resolution: %w", err)
	}

	remote := cloneURL
	argsPrefix := []string(nil)
	if probeOrigin {
		remote = "origin"
		argsPrefix = []string{"-C", targetPath}
	}

	for _, candidate := range strings.Split(configured, ",") {
		branch := strings.TrimSpace(candidate)
		if branch == "" {
			continue
		}
		if err := gitcmd.SanitizeBranchName(branch); err != nil {
			return "", fmt.Errorf("invalid configured branch name %q: %w", branch, err)
		}
		// Preserve checkoutBranch's ordered fallback semantics: HEAD stops
		// selection and keeps origin/HEAD as the update target.
		if branch == "HEAD" {
			return "", nil
		}

		exists, err := remoteBranchExists(ctx, argsPrefix, remote, branch, env)
		if err != nil {
			return "", err
		}
		if exists {
			logger.Debug("resolved configured branch: %s -> %s", targetPath, branch)
			return branch, nil
		}
		logger.Debug("configured branch not found on origin, trying next: %s -> %s", targetPath, branch)
	}

	return "", fmt.Errorf("none of the configured branches exist on origin: %s", configured)
}

type branchResolutionError struct {
	err       error
	retryable bool
}

func (e *branchResolutionError) Error() string { return e.err.Error() }

func (e *branchResolutionError) Unwrap() error { return e.err }

func resolveConfiguredBranchWithRetries(ctx context.Context, targetPath, cloneURL string, env []string, configured string, probeOrigin bool, logger repo.Logger, attempts int, sink ProgressSink, action Action) (string, error) {
	var err error
	for i := range attempts {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		branch, resolveErr := resolveConfiguredBranch(ctx, targetPath, cloneURL, env, configured, probeOrigin, logger)
		if resolveErr == nil {
			return branch, nil
		}
		err = resolveErr
		var resolutionErr *branchResolutionError
		if !errors.As(resolveErr, &resolutionErr) || !resolutionErr.retryable {
			return "", resolveErr
		}
		if i < attempts-1 {
			sink.OnProgress(action, fmt.Sprintf("retrying branch resolution (%d/%d): %v", i+1, attempts-1, err), 0)
			timer := time.NewTimer(time.Duration(i+1) * 300 * time.Millisecond)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return "", ctx.Err()
			case <-timer.C:
			}
		}
	}
	return "", err
}

func inspectTarget(targetPath string) (exists, isGitRepo bool, err error) {
	info, err := os.Stat(targetPath)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("inspect target repository %q: %w", targetPath, err)
	}
	if !info.IsDir() {
		return true, false, nil
	}
	_, err = os.Stat(filepath.Join(targetPath, ".git"))
	if err == nil || os.IsNotExist(err) {
		return true, err == nil, nil
	}
	return true, false, fmt.Errorf("inspect target repository %q: %w", targetPath, err)
}

// checkoutResolvedBranchBeforeUpdate first refreshes the exact remote-tracking
// ref, then checks it out. Fetch is safe before reset/pull/rebase and lets a
// single-branch checkout adopt a configured branch that is not yet local.
func checkoutResolvedBranchBeforeUpdate(ctx context.Context, targetPath, branch string, env []string, discardChanges bool) error {
	if err := gitcmd.SanitizeBranchName(branch); err != nil {
		return fmt.Errorf("invalid configured branch name %q: %w", branch, err)
	}
	usedElsewhere, err := branchUsedByAnotherWorktree(ctx, targetPath, branch)
	if err != nil {
		return err
	}
	if usedElsewhere {
		return fmt.Errorf("branch %q is checked out in another worktree", branch)
	}
	refspec := "+refs/heads/" + branch + ":refs/remotes/origin/" + branch
	cmd := exec.CommandContext(ctx, "git", "-C", targetPath, "fetch", "origin", refspec) // #nosec G204 -- target path and branch are validated or fixed argv values.
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fetch configured branch %q before update: %w (output: %s)", branch, err, strings.TrimSpace(string(output)))
	}
	localRef := "refs/heads/" + branch
	if localBranchExists(ctx, targetPath, localRef) {
		args := []string{"-C", targetPath, "switch"}
		if discardChanges {
			args = append(args, "--discard-changes")
		}
		args = append(args, branch)
		cmd = exec.CommandContext(ctx, "git", args...) // #nosec G204 -- branch is sanitized and known to be a local branch.
	} else {
		args := []string{"-C", targetPath, "checkout"}
		if discardChanges {
			args = append(args, "-f")
		}
		args = append(args, "-B", branch, "refs/remotes/origin/"+branch)
		cmd = exec.CommandContext(ctx, "git", args...) // #nosec G204 -- branch is sanitized and remote ref is constructed from it.
	}
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("switch to configured branch %q: %w (output: %s)", branch, err, strings.TrimSpace(string(output)))
	}
	current, err := symbolicBranch(ctx, targetPath)
	if err != nil {
		return err
	}
	if current != branch {
		return fmt.Errorf("configured branch checkout left HEAD on %q, want %q", current, branch)
	}
	return nil
}

func localBranchExists(ctx context.Context, targetPath, ref string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", targetPath, "show-ref", "--verify", "--quiet", ref) // #nosec G204 -- ref is built from a sanitized branch.
	return cmd.Run() == nil
}

func symbolicBranch(ctx context.Context, targetPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", targetPath, "symbolic-ref", "--quiet", "HEAD") // #nosec G204 -- targetPath is a repository path passed as one argv value; no shell is used and all other argv values are fixed.
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read current branch: %w", err)
	}
	return strings.TrimPrefix(strings.TrimSpace(string(output)), "refs/heads/"), nil
}

func branchUsedByAnotherWorktree(ctx context.Context, targetPath, branch string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", targetPath, "worktree", "list", "--porcelain") // #nosec G204 -- targetPath is a repository path passed as one argv value; no shell is used and all other argv values are fixed.
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("list worktrees: %w", err)
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		return false, fmt.Errorf("stat target worktree: %w", err)
	}
	var worktree string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			worktree = strings.TrimPrefix(line, "worktree ")
			continue
		}
		if line == "branch refs/heads/"+branch {
			worktreeInfo, statErr := os.Stat(worktree)
			if statErr != nil {
				return false, fmt.Errorf("stat worktree path: %w", statErr)
			}
			if !os.SameFile(targetInfo, worktreeInfo) {
				return true, nil
			}
		}
	}
	return false, nil
}

// remoteBranchExists checks exactly one branch ref. git ls-remote returns exit
// status 2 when the remote is reachable but the requested ref is absent; any
// other failure is surfaced rather than treated as a missing candidate.
func remoteBranchExists(ctx context.Context, argsPrefix []string, remote, branch string, env []string) (bool, error) {
	args := append([]string{}, argsPrefix...)
	args = append(args, "ls-remote", "--exit-code", "--heads", remote, "refs/heads/"+branch)
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- remote is fixed to origin or validated clone URL, and branch is sanitized.
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		return false, nil
	}
	return false, &branchResolutionError{err: fmt.Errorf("query origin branch %q failed: %w (output: %s)", branch, err, strings.TrimSpace(string(output))), retryable: true}
}

// checkoutBranch attempts to checkout the specified branch in the given repository path.
// Supports comma-separated branch list for fallback: "develop,master" tries develop first,
// then master if develop doesn't exist. If all specified branches fail, returns an error.
// Returns a success message and nil error on success, or an error on failure.
func checkoutBranch(ctx context.Context, repoPath, branch string, logger repo.Logger) (string, error) {
	// Support comma-separated branch list for fallback
	branches := strings.Split(branch, ",")
	var lastErr error

	for _, b := range branches {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}

		// Option-injection defense: these branch names come from config
		// (defaultBranch: "develop,master") and flow to git as bare positional
		// args in branchExists/checkout below. Reject values git could parse as
		// options (e.g. --upload-pack=…); skip to the next fallback on rejection.
		if err := gitcmd.SanitizeBranchName(b); err != nil {
			lastErr = fmt.Errorf("invalid branch name %q: %w", b, err)
			logger.Debug("branch name rejected, trying next: %s -> %s: %v", repoPath, b, lastErr)
			continue
		}
		if b == "HEAD" {
			return "kept current HEAD", nil
		}

		// Check if branch exists (local or remote)
		if branchExists(ctx, repoPath, b) {
			// Use exec.CommandContext to respect context cancellation/timeout
			cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "checkout", b) // #nosec G204 -- b passed here is validated by SanitizeBranchName; no shell is used.
			output, err := cmd.CombinedOutput()
			if err != nil {
				lastErr = fmt.Errorf("git checkout %s failed: %w (output: %s)", b, err, string(output))
				logger.Debug("branch checkout failed, trying next: %s -> %s: %v", repoPath, b, lastErr)
				continue
			}

			logger.Debug("branch checkout: %s -> %s", repoPath, b)
			return fmt.Sprintf("checked out %s", b), nil
		}
		logger.Debug("branch not found, trying next: %s -> %s", repoPath, b)
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("none of the specified branches exist: %s", branch)
}

// branchExists checks if a branch exists locally or as a remote tracking branch.
func branchExists(ctx context.Context, repoPath, branch string) bool {
	// Check local branch
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--verify", "--quiet", branch) // #nosec G204 -- branch is validated before this helper is called.
	if cmd.Run() == nil {
		return true
	}

	// Check remote tracking branch (origin/branch)
	cmd = exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--verify", "--quiet", "origin/"+branch) // #nosec G204 -- branch is validated before this helper is called.
	return cmd.Run() == nil
}

// addAdditionalRemotes configures additional git remotes in the given repository path.
// Returns a success message and nil error on success, or an error on failure.
func addAdditionalRemotes(ctx context.Context, repoPath string, remotes map[string]string, logger repo.Logger) (string, error) {
	if len(remotes) == 0 {
		return "", nil
	}

	var addedRemotes []string
	for remoteName, remoteURL := range remotes {
		if err := gitcmd.SanitizeRemoteName(remoteName); err != nil {
			return "", fmt.Errorf("invalid additional remote name %q: %w", remoteName, err)
		}
		if err := gitcmd.SanitizeURL(remoteURL); err != nil {
			return "", fmt.Errorf("invalid URL for additional remote %q: %w", remoteName, err)
		}
	}

	for remoteName, remoteURL := range remotes {
		// Check if remote already exists
		checkCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", remoteName) // #nosec G204 -- remoteName is validated and no shell is used.
		existingURL, err := checkCmd.Output()

		if err == nil && len(existingURL) > 0 {
			// Remote exists - update URL if different
			currentURL := string(existingURL)
			if currentURL != remoteURL+"\n" {
				updateCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "set-url", remoteName, remoteURL) // #nosec G204 -- validated remote name and URL are passed as separate argv values.
				if output, err := updateCmd.CombinedOutput(); err != nil {
					return "", fmt.Errorf("git remote set-url %s failed: %w (output: %s)", remoteName, err, string(output))
				}
				logger.Debug("updated remote: %s -> %s (in %s)", remoteName, remoteURL, repoPath)
				addedRemotes = append(addedRemotes, remoteName)
			}
		} else {
			// Remote doesn't exist - add it
			addCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "add", remoteName, remoteURL) // #nosec G204 -- validated remote name and URL are passed as separate argv values.
			if output, err := addCmd.CombinedOutput(); err != nil {
				return "", fmt.Errorf("git remote add %s failed: %w (output: %s)", remoteName, err, string(output))
			}
			logger.Debug("added remote: %s -> %s (in %s)", remoteName, remoteURL, repoPath)
			addedRemotes = append(addedRemotes, remoteName)
		}
	}

	if len(addedRemotes) > 0 {
		return fmt.Sprintf("configured %d remote(s)", len(addedRemotes)), nil
	}

	return "", nil
}
