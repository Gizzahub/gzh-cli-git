package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

var (
	commitFlags    BulkCommandFlags
	commitAll      string   // --all: common message for all repos
	commitMessages []string // -m, --message: per-repo messages
	commitYes      bool
	commitEdit     bool
	commitJSON     string // --json: inline JSON messages
	commitYAML     string // --yaml: inline YAML messages

	commitAllowConflicted bool // --allow-conflicted: commit repos with unmerged paths
)

// commitCmd represents the commit command.
var commitCmd = &cobra.Command{
	Use:   "commit [directory]",
	Short: "Commit changes across multiple repositories",
	Long: cliutil.QuickStartHelp(`  # Commit with per-repository messages (most common usage)
  gz-git commit -m "repo1:feat: add feature" -m "repo2:fix: bug fix"

  # Commit with same message for all repositories
  gz-git commit --all "chore: update dependencies"

  # Commit using inline JSON (Best for LLM pipelines and scripts)
  gz-git commit --json '{"repo1":"feat: add feature", "repo2":"fix: bug fix"}'

  # Commit using piped JSON or YAML (Best for workflow integration)
  cat messages.yaml | gz-git commit --yes

  # Interactive mode: edit messages in editor before committing
  gz-git commit -e

  # Scan and show dirty repos (preview only)
  gz-git commit

  # Skip confirmation
  gz-git commit --yes`) + cliutil.ExitCodesBulkHelp(),
	Args: cobra.MaximumNArgs(1),
	RunE: runCommit,
}

func init() {
	rootCmd.AddCommand(commitCmd)

	// Common bulk operation flags
	addBulkFlags(commitCmd, &commitFlags)

	// Commit-specific flags
	commitCmd.Flags().StringArrayVarP(&commitMessages, "message", "m", []string{}, "per-repository message in format 'repo:message' (can be repeated)")
	commitCmd.Flags().StringVar(&commitAll, "all", "", "common commit message for all repositories")
	commitCmd.Flags().BoolVarP(&commitYes, "yes", "y", false, "auto-approve without confirmation")
	commitCmd.Flags().BoolVarP(&commitEdit, "edit", "e", false, "edit messages in $EDITOR before committing")
	commitCmd.Flags().StringVar(&commitJSON, "json", "", `inline JSON with per-repo messages (e.g., '{"repo":"message"}')`)
	commitCmd.Flags().StringVar(&commitYAML, "yaml", "", `inline YAML with per-repo messages`)
	commitCmd.Flags().BoolVar(&commitAllowConflicted, "allow-conflicted", false, "commit repositories that still have unmerged paths (writes conflict markers into history)")
}

func runCommit(cmd *cobra.Command, args []string) error {
	// SIGINT/SIGTERM cancels the shared context so in-flight commits stop
	// gracefully (extracted into bulk_common's withInterruptCancel).
	ctx, cancel := withInterruptCancel(context.Background())
	defer cancel()

	config, err := prepareCommitRun(cmd, args)
	if err != nil {
		return err
	}
	client := repository.NewClient()

	// Scanning phase
	if shouldShowProgress(commitFlags.Format, quiet) {
		printScanningMessage(config.directory, commitFlags.Depth, commitFlags.Parallel, commitFlags.DryRun)
	}

	// Execute bulk commit (analysis phase if DryRun, otherwise commits)
	result, err := client.BulkCommit(ctx, config.options)
	if err != nil {
		return fmt.Errorf("bulk commit failed: %w", err)
	}

	// Display scan completion message
	if shouldShowProgress(commitFlags.Format, quiet) && result.TotalScanned == 0 {
		fmt.Printf("Scan complete: no repositories found\n")
	}

	// Apply custom messages from file or CLI
	if config.customMessages != nil {
		applyCustomMessages(result, config.customMessages)
	}

	// If -e flag is set, open editor for message editing then commit
	result, canceled, err := commitEditorResult(ctx, client, result, config.options)
	if err != nil {
		return err
	}
	if canceled {
		return nil
	}

	// Show hint for preview mode
	if config.previewOnly && result.TotalDirty > 0 && shouldShowProgress(commitFlags.Format, quiet) {
		fmt.Println("Hint: Use --yes (-y) to commit, or --edit (-e) to edit messages first")
	}

	// Display results
	if commitFlags.Format == "json" || !quiet {
		displayCommitResults(result)
	}

	// Conflicted repositories were deliberately left uncommitted. They must
	// reach the exit code, otherwise an unattended caller that only checks $?
	// records the refusal as a clean success.
	if result.TotalConflicted > 0 {
		fmt.Fprintf(os.Stderr,
			"\n%d repository(ies) skipped: unresolved merge conflicts. Resolve them and re-run, or pass --allow-conflicted.\n",
			result.TotalConflicted)
	}

	return errPartialFailure(
		result.TotalFailed+result.TotalConflicted,
		result.TotalDirty+result.TotalConflicted,
	)
}

type commitRunConfig struct {
	directory      string
	options        repository.BulkCommitOptions
	customMessages map[string]string
	previewOnly    bool
}

func prepareCommitRun(cmd *cobra.Command, args []string) (commitRunConfig, error) {
	directory, err := validateBulkDirectory(args)
	if err != nil {
		return commitRunConfig{}, err
	}
	if err := validateBulkDepth(cmd, commitFlags.Depth); err != nil {
		return commitRunConfig{}, err
	}
	if err := validateBulkFormat(commitFlags.Format); err != nil {
		return commitRunConfig{}, err
	}

	customMessages, err := loadCommitMessages()
	if err != nil {
		return commitRunConfig{}, err
	}

	opts := repository.BulkCommitOptions{
		Directory:         directory,
		Parallel:          commitFlags.Parallel,
		MaxDepth:          commitFlags.Depth,
		DryRun:            commitFlags.DryRun,
		Message:           commitAll,
		Yes:               commitYes,
		Verbose:           verbose,
		IncludeSubmodules: commitFlags.IncludeSubmodules,
		AllowConflicted:   commitAllowConflicted,
		IncludePattern:    commitFlags.Include,
		ExcludePattern:    commitFlags.Exclude,
		Logger:            createBulkLogger(verbose),
		ProgressCallback:  createProgressCallback("Analyzing", commitFlags.Format, quiet),
	}
	if customMessages != nil {
		opts.MessageGenerator = customCommitMessageGenerator(directory, customMessages)
	}

	previewOnly := !opts.Yes && !opts.DryRun && !commitEdit
	if previewOnly {
		opts.DryRun = true
	}
	return commitRunConfig{
		directory:      directory,
		options:        opts,
		customMessages: customMessages,
		previewOnly:    previewOnly,
	}, nil
}

func loadCommitMessages() (map[string]string, error) {
	var messages map[string]string
	stat, err := os.Stdin.Stat()
	switch {
	case err == nil && (stat.Mode()&os.ModeCharDevice) == 0:
		pipedData, _ := io.ReadAll(os.Stdin)
		if len(pipedData) > 0 {
			messages, err = parseJSONOrYAMLMessages(string(pipedData))
			if err != nil {
				return nil, fmt.Errorf("failed to parse piped data: %w", err)
			}
		}
	case commitJSON != "":
		messages, err = parseJSONMessages(commitJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to parse --json: %w", err)
		}
	case commitYAML != "":
		messages, err = parseYAMLMessages(commitYAML)
		if err != nil {
			return nil, fmt.Errorf("failed to parse --yaml: %w", err)
		}
	}

	if len(commitMessages) > 0 {
		if messages == nil {
			messages = make(map[string]string)
		}
		for _, raw := range commitMessages {
			repo, message, parseErr := parseRepoMessage(raw)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid --message format: %w", parseErr)
			}
			messages[repo] = message
		}
	}
	return messages, nil
}

func customCommitMessageGenerator(directory string, messages map[string]string) func(context.Context, string, []string) (string, error) {
	return func(_ context.Context, repoPath string, _ []string) (string, error) {
		relPath, err := filepath.Rel(directory, repoPath)
		if err != nil {
			relPath = filepath.Base(repoPath)
		}
		if relPath == "." {
			relPath = filepath.Base(directory)
		}
		if message, ok := messages[relPath]; ok {
			return message, nil
		}
		if message, ok := messages[filepath.Base(relPath)]; ok {
			return message, nil
		}
		if message, ok := messages[repoPath]; ok {
			return message, nil
		}
		return "", nil
	}
}

func commitEditorResult(ctx context.Context, client repository.Client, result *repository.BulkCommitResult, opts repository.BulkCommitOptions) (*repository.BulkCommitResult, bool, error) {
	if !commitEdit || result.TotalDirty == 0 {
		return result, false, nil
	}
	editedMessages, err := editMessagesInEditor(result)
	if err != nil {
		return nil, false, fmt.Errorf("editor failed: %w", err)
	}
	if editedMessages == nil {
		fmt.Println("Canceled (empty file).")
		return result, true, nil
	}
	applyCustomMessages(result, editedMessages)
	opts.DryRun = false
	opts.Yes = true
	opts.MessageGenerator = commitResultMessageGenerator(result)
	result, err = client.BulkCommit(ctx, opts)
	if err != nil {
		return nil, false, fmt.Errorf("bulk commit failed: %w", err)
	}
	return result, false, nil
}

func commitResultMessageGenerator(result *repository.BulkCommitResult) func(context.Context, string, []string) (string, error) {
	return func(_ context.Context, repoPath string, _ []string) (string, error) {
		for _, repo := range result.Repositories {
			if repo.Path == repoPath {
				if repo.Message != "" {
					return repo.Message, nil
				}
				return repo.SuggestedMessage, nil
			}
		}
		return "", nil
	}
}

// parseRepoMessage parses "repo:message" format.
func parseRepoMessage(input string) (repo, message string, err error) {
	before, after, ok := strings.Cut(input, ":")
	if !ok {
		return "", "", fmt.Errorf("expected format 'repo:message', got %q", input)
	}
	repo = strings.TrimSpace(before)
	message = strings.TrimSpace(after)
	if repo == "" || message == "" {
		return "", "", fmt.Errorf("repo and message cannot be empty in %q", input)
	}
	return repo, message, nil
}

// parseJSONMessages parses inline JSON string to message map.
func parseJSONMessages(jsonStr string) (map[string]string, error) {
	var messages map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &messages); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return messages, nil
}

// parseYAMLMessages parses inline YAML string to message map.
func parseYAMLMessages(yamlStr string) (map[string]string, error) {
	var messages map[string]string
	if err := yaml.Unmarshal([]byte(yamlStr), &messages); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	return messages, nil
}

// parseJSONOrYAMLMessages tries parsing JSON first, then YAML.
func parseJSONOrYAMLMessages(data string) (map[string]string, error) {
	var messages map[string]string
	if err := json.Unmarshal([]byte(data), &messages); err == nil {
		return messages, nil
	}
	if err := yaml.Unmarshal([]byte(data), &messages); err == nil {
		return messages, nil
	}
	return nil, fmt.Errorf("data is not valid payload (expected JSON or YAML)")
}

// applyCustomMessages applies custom messages to repository results.
func applyCustomMessages(result *repository.BulkCommitResult, messages map[string]string) {
	for i := range result.Repositories {
		repo := &result.Repositories[i]
		// Try to match by relative path or full path
		if msg, ok := messages[repo.RelativePath]; ok {
			repo.Message = msg
		} else if msg, ok := messages[filepath.Base(repo.RelativePath)]; ok {
			repo.Message = msg
		} else if msg, ok := messages[repo.Path]; ok {
			repo.Message = msg
		}
	}
}

// editMessagesInEditor opens an editor for bulk message editing.
// Returns nil if the user canceled (empty file).
func editMessagesInEditor(result *repository.BulkCommitResult) (map[string]string, error) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "gz-git-commit-*.txt")
	if err != nil {
		return nil, fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write template
	var content strings.Builder
	content.WriteString("# Bulk Commit Messages\n")
	content.WriteString("# Edit messages below. Lines starting with # are ignored.\n")
	content.WriteString("# Format: repository: commit message\n")
	content.WriteString("# Save and close to proceed. Delete all lines to cancel.\n")
	content.WriteString("#\n")

	for _, repo := range result.Repositories {
		if repo.Status != "dirty" && repo.Status != "would-commit" {
			continue
		}
		msg := repo.SuggestedMessage
		if repo.Message != "" {
			msg = repo.Message
		}
		fmt.Fprintf(&content, "%s: %s\n", repo.RelativePath, msg)
	}

	if _, err := tmpFile.WriteString(content.String()); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("cannot write temp file: %w", err)
	}
	tmpFile.Close()

	// Get editor from environment
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		// Try common editors in order of preference
		for _, candidate := range []string{"vim", "vi", "nano", "notepad"} {
			if _, err := exec.LookPath(candidate); err == nil {
				editor = candidate
				break
			}
		}
	}
	if editor == "" {
		return nil, fmt.Errorf("no editor found: set EDITOR or VISUAL environment variable, or install vim/nano")
	}

	// Verify editor exists (in case $EDITOR is set but invalid)
	editorPath, err := exec.LookPath(editor)
	if err != nil {
		return nil, fmt.Errorf("editor '%s' not found: %w (set EDITOR or VISUAL to a valid editor)", editor, err)
	}

	// Open editor
	cmd := exec.CommandContext(context.Background(), editorPath, tmpPath) // #nosec G702 -- editorPath is resolved from the user's trusted editor configuration and no shell is used.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Check for specific error types
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("editor exited with code %d: consider using --json instead", exitErr.ExitCode())
		}
		return nil, fmt.Errorf("failed to run editor '%s': %w", editor, err)
	}

	// Read edited content
	editedData, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read edited file: %w", err)
	}

	// Parse edited content
	messages := make(map[string]string)
	lines := strings.Split(string(editedData), "\n")
	hasContent := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse "repo: message" format
		before, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		repo := strings.TrimSpace(before)
		msg := strings.TrimSpace(after)

		if repo != "" && msg != "" {
			messages[repo] = msg
			hasContent = true
		}
	}

	if !hasContent {
		return nil, nil //nolint:nilnil // cancellation is represented by an empty result and no error.
	}

	return messages, nil
}

func displayCommitResults(result *repository.BulkCommitResult) {
	// JSON or LLM output mode
	if commitFlags.Format == "json" || commitFlags.Format == "llm" {
		displayCommitResultsStructured(result, commitFlags.Format)
		return
	}

	fmt.Println()
	fmt.Println("=== Bulk Commit Results ===")
	fmt.Printf("Total scanned:   %d repositories\n", result.TotalScanned)
	fmt.Printf("Total dirty:     %d repositories\n", result.TotalDirty)
	fmt.Printf("Total committed: %d repositories\n", result.TotalCommitted)
	fmt.Printf("Total skipped:   %d repositories\n", result.TotalSkipped)
	if result.TotalConflicted > 0 {
		fmt.Printf("Total conflicted: %d repositories (not committed)\n", result.TotalConflicted)
	}
	fmt.Printf("Total failed:    %d repositories\n", result.TotalFailed)
	fmt.Printf("Duration:        %s\n", result.Duration.Round(100_000_000)) // Round to 0.1s
	fmt.Println()

	// Display summary
	if len(result.Summary) > 0 {
		fmt.Println("Summary by status:")
		for status, count := range result.Summary {
			icon := getCommitStatusIcon(status)
			fmt.Printf("  %s %-15s %d\n", icon, status+":", count)
		}
		fmt.Println()
	}

	// Display individual results if not compact
	if commitFlags.Format != "compact" && len(result.Repositories) > 0 {
		fmt.Println("Repository details:")
		for _, repo := range result.Repositories {
			displayCommitRepositoryResult(repo)
		}
	}

	// Display only errors/committed in compact mode
	if commitFlags.Format == "compact" {
		hasIssues := false
		for _, repo := range result.Repositories {
			if repo.Status == "error" || repo.Status == "success" || repo.Status == "conflicted" {
				if !hasIssues && repo.Status != "success" {
					fmt.Println("Issues found:")
					hasIssues = true
				}
				displayCommitRepositoryResult(repo)
			}
		}
		if !hasIssues && result.TotalCommitted > 0 {
			fmt.Printf("✓ Successfully committed %d repositories\n", result.TotalCommitted)
		} else if result.TotalDirty == 0 {
			fmt.Println("✓ All repositories are clean")
		}
	}
}

func displayCommitRepositoryResult(repo repository.RepositoryCommitResult) {
	icon := getCommitStatusIcon(repo.Status)

	// Build compact one-line format: icon path (branch) status duration
	parts := []string{icon}

	// Path with branch
	pathPart := repo.RelativePath
	if repo.Branch != "" {
		pathPart += fmt.Sprintf(" (%s)", repo.Branch)
	}
	parts = append(parts, fmt.Sprintf("%-50s", pathPart))

	// Show status compactly
	statusStr := ""
	switch repo.Status {
	case "success":
		if repo.CommitHash != "" {
			statusStr = fmt.Sprintf("committed [%s]", repo.CommitHash)
		} else {
			statusStr = "committed"
		}
	case "clean":
		statusStr = "clean"
	case "dirty", "would-commit":
		statusStr = fmt.Sprintf("%d files changed", repo.FilesChanged)
	case "conflicted":
		statusStr = fmt.Sprintf("CONFLICT: %d unmerged", len(repo.ConflictedFiles))
	case "error":
		statusStr = "failed"
	case "skipped":
		statusStr = "skipped"
	default:
		statusStr = repo.Status
	}

	parts = append(parts, fmt.Sprintf("%-25s", statusStr))

	// Duration
	if repo.Duration > 0 {
		parts = append(parts, fmt.Sprintf("%6s", repo.Duration.Round(10_000_000)))
	}

	// Build output line safely
	line := "  " + parts[0] + " " + parts[1] + " " + parts[2]
	if len(parts) > 3 {
		line += " " + parts[3]
	}
	fmt.Println(line)

	// Conflicted repositories always list their unmerged paths, regardless of
	// --verbose: this is a refusal the user has to act on, and burying it was
	// exactly how the silent conflict-commit went unnoticed.
	if repo.Status == "conflicted" {
		for _, file := range repo.ConflictedFiles {
			fmt.Printf("    unmerged: %s\n", file)
		}
		fmt.Println("    → resolve the merge, then re-run (or pass --allow-conflicted)")
		return
	}

	// Show error details if present
	if repo.Error != nil && verbose {
		fmt.Printf("    Error: %v\n", repo.Error)
	}
}

func getCommitStatusIcon(status string) string {
	switch status {
	case "success":
		return "✓"
	case "clean":
		return "="
	case "dirty", "would-commit":
		return "⚠"
	case "conflicted":
		return "⊗"
	case "error":
		return "✗"
	case "skipped":
		return "⊘"
	default:
		return "•"
	}
}

// CommitJSONOutput represents the JSON output structure for commit command.
type CommitJSONOutput struct {
	TotalScanned    int                          `json:"total_scanned"`
	TotalDirty      int                          `json:"total_dirty"`
	TotalCommitted  int                          `json:"total_committed"`
	TotalSkipped    int                          `json:"total_skipped"`
	TotalConflicted int                          `json:"total_conflicted,omitempty"`
	TotalFailed     int                          `json:"total_failed"`
	DurationMs      int64                        `json:"duration_ms"`
	Summary         map[string]int               `json:"summary"`
	Repositories    []CommitRepositoryJSONOutput `json:"repositories"`
}

// CommitRepositoryJSONOutput represents a single repository in JSON output.
type CommitRepositoryJSONOutput struct {
	Path                  string   `json:"path"`
	Branch                string   `json:"branch,omitempty"`
	Status                string   `json:"status"`
	CommitHash            string   `json:"commit_hash,omitempty"`
	Message               string   `json:"message,omitempty"`
	SuggestedMessage      string   `json:"suggested_message,omitempty"`
	FilesChanged          int      `json:"files_changed,omitempty"`
	TrackedFilesChanged   int      `json:"tracked_files_changed,omitempty"`
	UntrackedFilesChanged int      `json:"untracked_files_changed,omitempty"`
	StagedFilesChanged    int      `json:"staged_files_changed,omitempty"`
	Additions             int      `json:"additions,omitempty"`
	Deletions             int      `json:"deletions,omitempty"`
	ChangedFiles          []string `json:"changed_files,omitempty"`
	ConflictedFiles       []string `json:"conflicted_files,omitempty"`
	DurationMs            int64    `json:"duration_ms,omitempty"`
	Error                 string   `json:"error,omitempty"`
}

func displayCommitResultsStructured(result *repository.BulkCommitResult, format string) {
	output := CommitJSONOutput{
		TotalScanned:    result.TotalScanned,
		TotalDirty:      result.TotalDirty,
		TotalCommitted:  result.TotalCommitted,
		TotalSkipped:    result.TotalSkipped,
		TotalConflicted: result.TotalConflicted,
		TotalFailed:     result.TotalFailed,
		DurationMs:      result.Duration.Milliseconds(),
		Summary:         result.Summary,
		Repositories:    make([]CommitRepositoryJSONOutput, 0, len(result.Repositories)),
	}

	for _, repo := range result.Repositories {
		repoOutput := CommitRepositoryJSONOutput{
			Path:                  repo.RelativePath,
			Branch:                repo.Branch,
			Status:                repo.Status,
			CommitHash:            repo.CommitHash,
			Message:               repo.Message,
			SuggestedMessage:      repo.SuggestedMessage,
			FilesChanged:          repo.FilesChanged,
			TrackedFilesChanged:   repo.TrackedFilesChanged,
			UntrackedFilesChanged: repo.UntrackedFilesChanged,
			StagedFilesChanged:    repo.StagedFilesChanged,
			Additions:             repo.Additions,
			Deletions:             repo.Deletions,
			ChangedFiles:          repo.ChangedFiles,
			ConflictedFiles:       repo.ConflictedFiles,
			DurationMs:            repo.Duration.Milliseconds(),
		}
		if repo.Error != nil {
			repoOutput.Error = repo.Error.Error()
		}
		output.Repositories = append(output.Repositories, repoOutput)
	}

	writeBulkOutput(format, output)
}
