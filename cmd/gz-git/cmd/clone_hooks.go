package cmd

import (
	"context"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/hooks"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/repository"
)

// parseCloneHooks parses hooks from a raw map interface.
func parseCloneHooks(raw map[string]any) *CloneHooks {
	parsedHooks := &CloneHooks{}

	if before, ok := raw["before"].([]any); ok {
		for _, b := range before {
			if s, ok := b.(string); ok && s != "" {
				parsedHooks.Before = append(parsedHooks.Before, s)
			}
		}
	}

	if after, ok := raw["after"].([]any); ok {
		for _, a := range after {
			if s, ok := a.(string); ok && s != "" {
				parsedHooks.After = append(parsedHooks.After, s)
			}
		}
	}

	if len(parsedHooks.Before) == 0 && len(parsedHooks.After) == 0 {
		return nil
	}

	return parsedHooks
}

// mergeHooks merges group-level and repo-level hooks.
// Repo hooks are appended after group hooks.
func mergeHooks(groupHooks, repoHooks *CloneHooks) *CloneHooks {
	if groupHooks == nil && repoHooks == nil {
		return nil
	}

	merged := &CloneHooks{}

	if groupHooks != nil {
		merged.Before = append(merged.Before, groupHooks.Before...)
		merged.After = append(merged.After, groupHooks.After...)
	}

	if repoHooks != nil {
		merged.Before = append(merged.Before, repoHooks.Before...)
		merged.After = append(merged.After, repoHooks.After...)
	}

	return merged
}

// executeHooks runs hook commands in the specified directory.
// Returns error if any hook fails (marks repo as failed per user decision).
// Uses direct exec without shell for security (no pipes, redirects, variables).
//
// This function delegates to pkg/hooks.ExecuteCommands for the actual execution.
func executeHooks(ctx context.Context, commands []string, workDir string, logger repository.Logger) error {
	// Adapt repository.Logger to hooks.Logger interface
	var hookLogger hooks.Logger
	if logger != nil {
		hookLogger = &loggerAdapter{logger}
	}
	return hooks.ExecuteCommands(ctx, commands, workDir, hookLogger)
}

// loggerAdapter adapts repository.Logger to hooks.Logger interface.
type loggerAdapter struct {
	logger repository.Logger
}

func (a *loggerAdapter) Info(msg string, keysAndValues ...any) {
	a.logger.Info(msg, keysAndValues...)
}
