// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/config"
)

// Logger interface for hook execution logging.
type Logger interface {
	Info(msg string, keysAndValues ...any)
}

// DefaultTimeout is the default timeout for hook commands (30 seconds).
const DefaultTimeout = 30 * time.Second

// Execute runs hook commands in the specified directory.
// It runs hooks for the specified phase ("before" or "after").
// Returns error if any hook fails (marks operation as failed).
// Uses direct exec without shell for security (no pipes, redirects, variables).
func Execute(ctx context.Context, hooks *config.Hooks, phase, workDir string, logger Logger) error {
	if hooks == nil {
		return nil
	}

	var commands []string
	switch phase {
	case "before":
		commands = hooks.Before
	case "after":
		commands = hooks.After
	default:
		return fmt.Errorf("invalid hook phase %q: must be 'before' or 'after'", phase)
	}

	return ExecuteCommands(ctx, commands, workDir, logger)
}

// ExecuteCommands runs a list of commands in the specified directory.
// Returns error if any command fails.
func ExecuteCommands(ctx context.Context, commands []string, workDir string, logger Logger) error {
	if len(commands) == 0 {
		return nil
	}

	// Validate working directory exists
	if _, err := os.Stat(workDir); err != nil {
		return fmt.Errorf("hook working directory does not exist: %s", workDir)
	}

	for _, cmd := range commands {
		args := ParseCommand(cmd)
		if len(args) == 0 {
			continue
		}

		// Create context with timeout
		hookCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)

		execCmd := exec.CommandContext(hookCtx, args[0], args[1:]...) // #nosec G204 -- hook argv is an explicit configured argument vector; no shell is used.
		execCmd.Dir = workDir
		execCmd.Env = os.Environ()

		output, err := execCmd.CombinedOutput()
		cancel()

		if err != nil {
			return fmt.Errorf("hook %q failed: %w (output: %s)", cmd, err, strings.TrimSpace(string(output)))
		}

		if logger != nil && len(output) > 0 {
			logger.Info("hook completed", "command", cmd, "output", strings.TrimSpace(string(output)))
		}
	}

	return nil
}

// ParseCommand splits a hook command string into executable and arguments.
// Supports simple quoting but NOT shell features (pipes, redirects, variables).
// This is intentional for security - use scripts for complex commands.
//
// Examples:
//
//	"make build" → ["make", "build"]
//	"echo 'hello world'" → ["echo", "hello world"]
//	"cmd \"arg with spaces\"" → ["cmd", "arg with spaces"]
func ParseCommand(cmd string) []string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}

	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range cmd {
		switch {
		case inQuote:
			if r == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			inQuote = true
			quoteChar = r
		case r == ' ' || r == '\t':
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// Merge combines global and workspace-level hooks.
// Global hooks run first, then workspace hooks.
// Returns nil if both inputs are nil.
func Merge(global, local *config.Hooks) *config.Hooks {
	if global == nil && local == nil {
		return nil
	}

	merged := &config.Hooks{}

	if global != nil {
		merged.Before = append(merged.Before, global.Before...)
		merged.After = append(merged.After, global.After...)
	}

	if local != nil {
		merged.Before = append(merged.Before, local.Before...)
		merged.After = append(merged.After, local.After...)
	}

	// Return nil if both are empty
	if len(merged.Before) == 0 && len(merged.After) == 0 {
		return nil
	}

	return merged
}

// HasUnsafeCharacters checks if a command contains potentially dangerous characters.
// Returns true if pipes, redirects, or variable expansion are detected.
func HasUnsafeCharacters(cmd string) bool {
	dangerousChars := []string{"|", ">", "<", ">>", "<<", "$(", "`", "&&", "||", ";"}
	for _, char := range dangerousChars {
		if strings.Contains(cmd, char) {
			return true
		}
	}
	return false
}

// ValidateCommands checks if all commands in a hook list are safe.
// Returns an error describing which command has unsafe characters.
func ValidateCommands(commands []string) error {
	for _, cmd := range commands {
		if HasUnsafeCharacters(cmd) {
			return fmt.Errorf("command %q contains shell special characters (pipes, redirects, etc.) - use a script instead", cmd)
		}
	}
	return nil
}

// Validate checks if hooks configuration is safe.
// Returns error if any command contains unsafe shell characters.
func Validate(hooks *config.Hooks) error {
	if hooks == nil {
		return nil
	}

	if err := ValidateCommands(hooks.Before); err != nil {
		return fmt.Errorf("before hooks: %w", err)
	}

	if err := ValidateCommands(hooks.After); err != nil {
		return fmt.Errorf("after hooks: %w", err)
	}

	return nil
}
