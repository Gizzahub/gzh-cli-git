package commit

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gizzahub/gzh-cli-git/internal/gitcmd"
	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// Generator generates commit messages automatically.
type Generator interface {
	// Generate creates a commit message from changes.
	Generate(ctx context.Context, repo *repository.Repository, opts GenerateOptions) (string, error)

	// Suggest suggests commit type and scope.
	Suggest(ctx context.Context, changes *DiffSummary) (*Suggestion, error)
}

// GenerateOptions configures message generation.
type GenerateOptions struct {
	Template    *Template
	Interactive bool // Ask user for clarifications
	MaxLength   int
}

// DiffSummary summarizes git diff.
type DiffSummary struct {
	FilesChanged  int
	Insertions    int
	Deletions     int
	ModifiedFiles []string
	AddedFiles    []string
	DeletedFiles  []string
}

// Suggestion suggests commit metadata.
type Suggestion struct {
	Type        string  // feat, fix, docs, etc.
	Scope       string  // Inferred scope
	Description string  // Generated description
	Confidence  float64 // 0.0 - 1.0
}

// generator implements Generator.
type generator struct {
	executor    *gitcmd.Executor
	templateMgr TemplateManager
}

// NewGenerator creates a new Generator.
func NewGenerator() Generator {
	return &generator{
		executor:    gitcmd.NewExecutor(),
		templateMgr: NewTemplateManager(),
	}
}

// NewGeneratorWithDeps creates a new Generator with custom dependencies.
func NewGeneratorWithDeps(executor *gitcmd.Executor, templateMgr TemplateManager) Generator {
	return &generator{
		executor:    executor,
		templateMgr: templateMgr,
	}
}

// Generate creates a commit message from changes.
func (g *generator) Generate(ctx context.Context, repo *repository.Repository, opts GenerateOptions) (string, error) {
	if repo == nil {
		return "", fmt.Errorf("repository cannot be nil")
	}

	// Set defaults
	if opts.Template == nil {
		tmpl, err := g.templateMgr.Load(ctx, "conventional")
		if err != nil {
			return "", fmt.Errorf("failed to load default template: %w", err)
		}
		opts.Template = tmpl
	}
	if opts.MaxLength == 0 {
		opts.MaxLength = 72
	}

	// Get diff summary
	summary, err := g.getDiffSummary(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("failed to get diff summary: %w", err)
	}

	// Check if there are changes
	if summary.FilesChanged == 0 {
		return "", ErrNoChanges
	}

	// Suggest commit type and scope
	suggestion, err := g.Suggest(ctx, summary)
	if err != nil {
		return "", fmt.Errorf("failed to generate suggestion: %w", err)
	}

	// Build template values
	values := map[string]string{
		"Type":        suggestion.Type,
		"Description": suggestion.Description,
	}

	// Add scope if available
	if suggestion.Scope != "" {
		values["Scope"] = suggestion.Scope
	}

	// Render template
	message, err := g.templateMgr.Render(ctx, opts.Template, values)
	if err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return message, nil
}

// Suggest suggests commit type and scope.
func (g *generator) Suggest(ctx context.Context, changes *DiffSummary) (*Suggestion, error) {
	if changes == nil {
		return nil, fmt.Errorf("changes cannot be nil")
	}

	suggestion := &Suggestion{
		Type:       "chore",
		Scope:      "",
		Description: "update files",
		Confidence: 0.5,
	}

	// Analyze file patterns to determine type
	suggestion.Type, suggestion.Confidence = g.inferType(changes)

	// Infer scope from file paths
	suggestion.Scope = g.inferScope(changes)

	// Generate description
	suggestion.Description = g.generateDescription(changes, suggestion.Type)

	return suggestion, nil
}

// getDiffSummary gets a summary of uncommitted changes.
func (g *generator) getDiffSummary(ctx context.Context, repo *repository.Repository) (*DiffSummary, error) {
	// Get status to find changed files
	result, err := g.executor.Run(ctx, repo.Path, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	summary := &DiffSummary{
		ModifiedFiles: []string{},
		AddedFiles:    []string{},
		DeletedFiles:  []string{},
	}

	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		if len(line) < 4 {
			continue
		}

		status := strings.TrimSpace(line[0:2])
		filepath := strings.TrimSpace(line[3:])

		switch {
		case strings.Contains(status, "A"):
			summary.AddedFiles = append(summary.AddedFiles, filepath)
		case strings.Contains(status, "D"):
			summary.DeletedFiles = append(summary.DeletedFiles, filepath)
		case strings.Contains(status, "M"):
			summary.ModifiedFiles = append(summary.ModifiedFiles, filepath)
		case strings.Contains(status, "R"):
			summary.ModifiedFiles = append(summary.ModifiedFiles, filepath)
		default:
			summary.ModifiedFiles = append(summary.ModifiedFiles, filepath)
		}
	}

	summary.FilesChanged = len(summary.AddedFiles) + len(summary.ModifiedFiles) + len(summary.DeletedFiles)

	// Get insertions/deletions from diff --stat
	diffResult, err := g.executor.Run(ctx, repo.Path, "diff", "--cached", "--stat")
	if err == nil {
		summary.Insertions, summary.Deletions = g.parseStats(diffResult.Stdout)
	}

	return summary, nil
}

// inferType infers commit type from file changes.
func (g *generator) inferType(changes *DiffSummary) (string, float64) {
	allFiles := append(append(changes.ModifiedFiles, changes.AddedFiles...), changes.DeletedFiles...)

	// Count files by category
	testFiles := 0
	docFiles := 0
	configFiles := 0
	codeFiles := 0

	for _, file := range allFiles {
		lower := strings.ToLower(file)
		switch {
		case strings.Contains(lower, "test"):
			testFiles++
		case strings.HasSuffix(lower, ".md"):
			docFiles++
		case strings.HasSuffix(lower, "readme"):
			docFiles++
		case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"),
			strings.HasSuffix(lower, ".json"), strings.HasSuffix(lower, ".toml"):
			configFiles++
		default:
			codeFiles++
		}
	}

	// Determine type based on file categories
	confidence := 0.7

	// Test files
	if testFiles > 0 && testFiles >= len(allFiles)/2 {
		return "test", 0.8
	}

	// Documentation
	if docFiles > 0 && docFiles >= len(allFiles)/2 {
		return "docs", 0.9
	}

	// Configuration
	if configFiles > 0 && configFiles >= len(allFiles)/2 {
		return "chore", 0.7
	}

	// New features (adding more files than deleting)
	if len(changes.AddedFiles) > len(changes.DeletedFiles) {
		return "feat", confidence
	}

	// Bug fixes (more modifications than additions)
	if len(changes.ModifiedFiles) > len(changes.AddedFiles) {
		return "fix", confidence
	}

	// Refactoring (balanced changes)
	if len(changes.ModifiedFiles) > 0 {
		return "refactor", 0.6
	}

	return "chore", 0.5
}

// inferScope infers scope from file paths.
func (g *generator) inferScope(changes *DiffSummary) string {
	if changes.FilesChanged == 0 {
		return ""
	}

	allFiles := append(append(changes.ModifiedFiles, changes.AddedFiles...), changes.DeletedFiles...)

	// Count directories
	dirCount := make(map[string]int)
	for _, file := range allFiles {
		dir := filepath.Dir(file)
		parts := strings.Split(dir, "/")

		// Use first meaningful directory
		if len(parts) > 0 {
			firstDir := parts[0]
			if firstDir != "." {
				dirCount[firstDir]++
			}
		}

		// Also count second level for specific modules
		if len(parts) > 1 {
			secondDir := parts[1]
			dirCount[secondDir]++
		}
	}

	// Find most common directory (prefer alphabetically first when counts equal for determinism)
	maxCount := 0
	scope := ""
	for dir, count := range dirCount {
		if count > maxCount || (count == maxCount && (scope == "" || dir < scope)) {
			maxCount = count
			scope = dir
		}
	}

	// Clean up scope
	scope = strings.TrimPrefix(scope, "pkg/")
	scope = strings.TrimPrefix(scope, "internal/")
	scope = strings.TrimPrefix(scope, "cmd/")

	return scope
}

// generateDescription generates a commit description.
func (g *generator) generateDescription(changes *DiffSummary, commitType string) string {
	fileCount := changes.FilesChanged

	// Generate description based on type and changes
	switch commitType {
	case "feat":
		if len(changes.AddedFiles) == 1 {
			fileName := filepath.Base(changes.AddedFiles[0])
			return fmt.Sprintf("add %s", strings.TrimSuffix(fileName, filepath.Ext(fileName)))
		}
		return fmt.Sprintf("add new features (%d files)", fileCount)

	case "fix":
		if len(changes.ModifiedFiles) == 1 {
			fileName := filepath.Base(changes.ModifiedFiles[0])
			return fmt.Sprintf("fix %s", strings.TrimSuffix(fileName, filepath.Ext(fileName)))
		}
		return fmt.Sprintf("fix bugs (%d files)", fileCount)

	case "docs":
		if len(changes.ModifiedFiles) == 1 {
			fileName := filepath.Base(changes.ModifiedFiles[0])
			return fmt.Sprintf("update %s", strings.ToLower(strings.TrimSuffix(fileName, filepath.Ext(fileName))))
		}
		return "update documentation"

	case "test":
		return fmt.Sprintf("add tests (%d files)", fileCount)

	case "refactor":
		if len(changes.ModifiedFiles) == 1 {
			fileName := filepath.Base(changes.ModifiedFiles[0])
			return fmt.Sprintf("refactor %s", strings.TrimSuffix(fileName, filepath.Ext(fileName)))
		}
		return fmt.Sprintf("refactor code (%d files)", fileCount)

	case "chore":
		return fmt.Sprintf("update configuration (%d files)", fileCount)

	default:
		return fmt.Sprintf("update files (%d files)", fileCount)
	}
}

// parseStats parses git diff --stat output.
func (g *generator) parseStats(output string) (insertions, deletions int) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Look for summary line like "1 file changed, 10 insertions(+), 5 deletions(-)"
		if strings.Contains(line, "changed") {
			parts := strings.Split(line, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.Contains(part, "insertion") {
					fmt.Sscanf(part, "%d", &insertions)
				}
				if strings.Contains(part, "deletion") {
					fmt.Sscanf(part, "%d", &deletions)
				}
			}
		}
	}
	return
}
