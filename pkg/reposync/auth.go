// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// AuthConfig holds authentication settings for git operations.
type AuthConfig struct {
	// Token is used for HTTPS clone URL injection
	Token string

	// Provider is the forge provider type (github, gitlab, gitea)
	// Used to determine the correct token format for URL injection
	Provider string

	// SSHKeyPath is the path to SSH private key file (priority)
	SSHKeyPath string

	// SSHKeyContent is the SSH private key content (used if SSHKeyPath is empty)
	SSHKeyContent string

	// SSHPort is the custom SSH port (0 = default)
	SSHPort int
}

// AuthResult contains the result of authentication setup.
type AuthResult struct {
	// CloneURL is the modified clone URL (with token injected for HTTPS)
	CloneURL string

	// Env contains environment variables to set for git commands
	Env []string

	// TempKeyPath is the path to temporary SSH key file (if created from content)
	// Caller should display warning to user about cleanup
	TempKeyPath string

	// Warnings contains non-fatal warnings (e.g., temp file cleanup reminder)
	Warnings []string
}

// PrepareAuth prepares authentication for a git clone operation.
// It modifies the clone URL for HTTPS (token injection) and sets up
// environment variables for SSH (GIT_SSH_COMMAND).
//
// Priority:
//  1. If auth config has token/key -> use it
//  2. Otherwise -> fallback to system defaults (no modification)
func PrepareAuth(cloneURL string, auth AuthConfig) (*AuthResult, error) {
	result := &AuthResult{
		CloneURL: cloneURL,
		Env:      []string{},
		Warnings: []string{},
	}

	// Determine protocol from URL
	isSSH := isSSHURL(cloneURL)

	if isSSH {
		// SSH authentication
		if err := prepareSSHAuth(result, auth); err != nil {
			return nil, fmt.Errorf("SSH auth setup failed: %w", err)
		}
	} else {
		// HTTPS authentication
		if err := prepareHTTPSAuth(result, auth); err != nil {
			return nil, fmt.Errorf("HTTPS auth setup failed: %w", err)
		}
	}

	return result, nil
}

// isSSHURL checks if the URL is an SSH URL.
func isSSHURL(cloneURL string) bool {
	// SSH URL formats:
	// - git@github.com:user/repo.git
	// - ssh://git@github.com/user/repo.git
	// - ssh://git@github.com:2224/user/repo.git
	if strings.HasPrefix(cloneURL, "ssh://") {
		return true
	}
	if strings.Contains(cloneURL, "@") && strings.Contains(cloneURL, ":") {
		// Check it's not https:// with port
		if !strings.HasPrefix(cloneURL, "http://") && !strings.HasPrefix(cloneURL, "https://") {
			return true
		}
	}
	return false
}

// prepareHTTPSAuth injects token into HTTPS URL if available.
func prepareHTTPSAuth(result *AuthResult, auth AuthConfig) error {
	if auth.Token == "" {
		// No token configured, fallback to system credential helper
		return nil
	}

	// Parse and modify URL
	modifiedURL, err := injectTokenToURL(result.CloneURL, auth.Token, auth.Provider)
	if err != nil {
		return err
	}

	result.CloneURL = modifiedURL
	return nil
}

// injectTokenToURL injects token into HTTPS URL based on provider.
//
// Provider-specific formats:
//   - GitLab: https://oauth2:TOKEN@gitlab.com/...
//   - GitHub: https://x-access-token:TOKEN@github.com/...
//   - Gitea:  https://TOKEN@gitea.com/...
func injectTokenToURL(cloneURL, token, provider string) (string, error) {
	// Skip SSH URLs (not parseable as standard URLs)
	if isSSHURL(cloneURL) {
		return cloneURL, nil
	}

	parsed, err := url.Parse(cloneURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Only modify http/https URLs
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return cloneURL, nil
	}

	// Determine username based on provider
	var username string
	switch strings.ToLower(provider) {
	case "gitlab":
		username = "oauth2"
	case "github":
		username = "x-access-token"
	case "gitea":
		// Gitea uses token directly without username prefix
		username = ""
	default:
		// Default to oauth2 for unknown providers
		username = "oauth2"
	}

	// Set user info
	if username != "" {
		parsed.User = url.UserPassword(username, token)
	} else {
		parsed.User = url.User(token)
	}

	return parsed.String(), nil
}

// prepareSSHAuth sets up SSH authentication via GIT_SSH_COMMAND.
func prepareSSHAuth(result *AuthResult, auth AuthConfig) error {
	var keyPath string

	// Priority: SSHKeyPath > SSHKeyContent > system default
	if auth.SSHKeyPath != "" {
		// Expand home directory
		expanded, err := expandHomePath(auth.SSHKeyPath)
		if err != nil {
			return fmt.Errorf("invalid SSH key path: %w", err)
		}

		// Verify file exists
		if _, err := os.Stat(expanded); os.IsNotExist(err) {
			return fmt.Errorf("SSH key file not found: %s", expanded)
		}

		keyPath = expanded
	} else if auth.SSHKeyContent != "" {
		// Create temporary file for SSH key content
		tempPath, err := createTempSSHKey(auth.SSHKeyContent)
		if err != nil {
			return fmt.Errorf("failed to create temp SSH key: %w", err)
		}

		keyPath = tempPath
		result.TempKeyPath = tempPath
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Temporary SSH key created at: %s (will be removed after the git operation)", tempPath))
	}

	// If no key configured, fallback to system default
	if keyPath == "" {
		return nil
	}

	// Build GIT_SSH_COMMAND
	sshCommand := buildSSHCommand(keyPath, auth.SSHPort)
	result.Env = append(result.Env, "GIT_SSH_COMMAND="+sshCommand)

	return nil
}

// buildSSHCommand builds the GIT_SSH_COMMAND value.
func buildSSHCommand(keyPath string, sshPort int) string {
	// Base command with key and IdentitiesOnly
	// IdentitiesOnly=yes prevents ssh from trying other keys
	// GIT_SSH_COMMAND is evaluated by a command shell. Quote the key path as
	// one argument so spaces and shell metacharacters cannot alter the command.
	cmd := fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new", quoteShellArg(keyPath))

	// Add custom port if specified
	if sshPort > 0 && sshPort != 22 {
		cmd += fmt.Sprintf(" -p %d", sshPort)
	}

	return cmd
}

// quoteShellArg quotes one argument in the command language used to execute
// GIT_SSH_COMMAND. Git for Unix-like systems uses a POSIX shell; Git for
// Windows uses the Windows command-line quoting rules for native commands.
// Keeping this at the boundary is important because key paths come from user
// configuration and are otherwise interpreted as command syntax.
func quoteShellArg(arg string) string {
	if runtime.GOOS == "windows" {
		return quoteWindowsCommandLineArg(arg)
	}

	// A single-quoted POSIX argument treats every byte literally. The only
	// character that needs escaping is a single quote itself.
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

// quoteWindowsCommandLineArg applies the quoting rules consumed by the
// Windows C runtime. Git for Windows passes GIT_SSH_COMMAND to a native SSH
// executable, so this keeps paths containing whitespace or quotes in one
// argument without relying on shell interpolation.
func quoteWindowsCommandLineArg(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\"") {
		return arg
	}

	var quoted strings.Builder
	quoted.WriteByte('"')
	backslashes := 0
	for _, char := range arg {
		switch char {
		case '\\':
			backslashes++
		case '"':
			quoted.WriteString(strings.Repeat("\\", backslashes*2+1))
			quoted.WriteByte('"')
			backslashes = 0
		default:
			quoted.WriteString(strings.Repeat("\\", backslashes))
			quoted.WriteRune(char)
			backslashes = 0
		}
	}
	quoted.WriteString(strings.Repeat("\\", backslashes*2))
	quoted.WriteByte('"')
	return quoted.String()
}

// createTempSSHKey creates a temporary file containing the SSH key content.
// The file is created with 0600 permissions in the system temp directory.
func createTempSSHKey(content string) (string, error) {
	// Create temp directory for gz-git keys if not exists
	tempDir := filepath.Join(os.TempDir(), "gz-git-keys")
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Create temp file
	tempFile, err := os.CreateTemp(tempDir, "ssh-key-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	cleanupOnError := func(cause error) (string, error) {
		if closeErr := tempFile.Close(); closeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("close temporary SSH key: %w", closeErr))
		}
		if removeErr := os.Remove(tempFile.Name()); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			cause = errors.Join(cause, fmt.Errorf("remove temporary SSH key: %w", removeErr))
		}
		return "", cause
	}

	// Set restrictive permissions (required by SSH)
	if err := os.Chmod(tempFile.Name(), 0o600); err != nil {
		return cleanupOnError(fmt.Errorf("failed to set file permissions: %w", err))
	}

	// Write key content
	if _, err := tempFile.WriteString(content); err != nil {
		return cleanupOnError(fmt.Errorf("failed to write key content: %w", err))
	}

	// Ensure content ends with newline (required by SSH)
	if !strings.HasSuffix(content, "\n") {
		if _, err := tempFile.WriteString("\n"); err != nil {
			return cleanupOnError(fmt.Errorf("failed to write newline: %w", err))
		}
	}
	if err := tempFile.Close(); err != nil {
		return cleanupOnError(fmt.Errorf("failed to close temporary SSH key: %w", err))
	}

	return tempFile.Name(), nil
}

func removeTempSSHKey(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove temporary SSH key %s: %w", path, err)
	}
	return nil
}

// expandHomePath expands ~ to home directory.
func expandHomePath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, path[1:]), nil
}

// MaskTokenInURL masks the token in a URL for safe logging.
func MaskTokenInURL(urlStr string) string {
	// Skip non-HTTP URLs
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return urlStr
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}

	if parsed.User == nil {
		return urlStr
	}

	// Reconstruct URL with masked credentials
	// Format: scheme://user:pass@host/path or scheme://user@host/path
	username := parsed.User.Username()
	_, hasPass := parsed.User.Password()

	var maskedUserInfo string
	if hasPass {
		maskedUserInfo = username + ":***"
	} else if username != "" {
		maskedUserInfo = "***"
	}

	// Build the masked URL manually to avoid URL encoding of ***
	result := parsed.Scheme + "://"
	if maskedUserInfo != "" {
		result += maskedUserInfo + "@"
	}
	result += parsed.Host + parsed.RequestURI()

	return result
}
