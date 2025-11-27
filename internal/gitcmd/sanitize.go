package gitcmd

import (
	"fmt"
	"regexp"
	"strings"
)

// Dangerous patterns that could enable command injection or path traversal.
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[;&|><$]`),         // Command separators and redirections
	regexp.MustCompile(`\$\(`),             // Command substitution $(...)
	regexp.MustCompile("`"),                // Backtick command substitution
	regexp.MustCompile(`\.\./`),            // Path traversal (relative)
	regexp.MustCompile(`^/(?:etc|usr|bin|sbin)/`), // System directories
	regexp.MustCompile(`\x00`),             // Null bytes
	regexp.MustCompile(`\r|\n`),            // Newlines (could break parsing)
}

// Safe Git flags that are known to be secure.
// This is a whitelist approach - only these flags are allowed.
var safeGitFlags = map[string]bool{
	// Common flags
	"--help":    true,
	"--version": true,
	"--verbose": true,
	"--quiet":   true,

	// Repository flags
	"--git-dir":   true,
	"--work-tree": true,
	"--bare":      true,

	// Clone flags
	"--branch":        true,
	"--depth":         true,
	"--single-branch": true,
	"--no-single-branch": true,
	"--recursive":     true,
	"--shallow-since": true,
	"--shallow-exclude": true,

	// Status flags
	"--porcelain":  true,
	"--short":      true,
	"--long":       true,
	"--untracked-files": true,
	"--ignored":    true,

	// Log flags
	"--oneline":    true,
	"--graph":      true,
	"--decorate":   true,
	"--all":        true,
	"--stat":       true,
	"--shortstat":  true,
	"--format":     true,
	"--pretty":     true,
	"--since":      true,
	"--until":      true,
	"--author":     true,
	"--committer":  true,
	"--max-count":  true,
	"--follow":     true,
	"--date":       true,

	// Commit flags
	"--message":    true,
	"--amend":      true,
	"--no-verify":  true,
	"--allow-empty": true,

	// Fetch/Pull/Push flags
	"--force":      true,
	"--dry-run":    true,
	"--tags":       true,
	"--no-tags":    true,
	"--prune":      true,
	"--set-upstream": true,

	// Merge/Rebase flags
	"--ff":         true,
	"--no-ff":      true,
	"--ff-only":    true,
	"--squash":     true,
	"--abort":      true,
	"--continue":   true,
	"--skip":       true,

	// Diff flags
	"--cached":     true,
	"--staged":     true,
	"--name-only":  true,
	"--name-status": true,
	"--numstat":    true,

	// Reset flags
	"--hard":       true,
	"--soft":       true,
	"--mixed":      true,

	// Remote flags
	"--add":        true,
	"--remove":     true,
	"--rename":     true,

	// Branch flags
	"--delete":     true,
	"--force-delete": true,
	"--list":       true,
	"--remote":     true,
	"--merged":     true,
	"--no-merged":  true,

	// Other safe flags
	"--abbrev-ref": true,
	"--show-toplevel": true,
	"--is-inside-work-tree": true,
}

// SanitizeArgs validates and sanitizes Git command arguments.
// This prevents command injection and other security issues.
//
// Returns an error if any argument contains dangerous patterns.
// Returns the sanitized arguments if all checks pass.
func SanitizeArgs(args []string) ([]string, error) {
	if len(args) == 0 {
		return args, nil
	}

	sanitized := make([]string, 0, len(args))

	for i, arg := range args {
		// Allow pipes and special characters in --format= values
		// These are safe because they're passed directly to git's format parser
		isFormatValue := strings.HasPrefix(arg, "--format=") || strings.HasPrefix(arg, "--pretty=")

		// Check for dangerous patterns (skip for format values)
		if !isFormatValue {
			for _, pattern := range dangerousPatterns {
				if pattern.MatchString(arg) {
					return nil, fmt.Errorf("argument %d contains dangerous pattern: %s", i, arg)
				}
			}
		}

		// Validate flags
		if strings.HasPrefix(arg, "-") {
			if err := validateFlag(arg); err != nil {
				return nil, fmt.Errorf("argument %d: %w", i, err)
			}
		}

		// Trim and add to sanitized list
		sanitized = append(sanitized, strings.TrimSpace(arg))
	}

	return sanitized, nil
}

// validateFlag checks if a flag is in the safe list.
// Flags with values (e.g., --branch=main) are also validated.
func validateFlag(flag string) error {
	// Allow the special '--' separator (used to separate flags from paths)
	if flag == "--" {
		return nil
	}

	// Extract flag name (before '=' if present)
	flagName := flag
	if idx := strings.Index(flag, "="); idx != -1 {
		flagName = flag[:idx]
	}

	// Check if flag is in whitelist
	if !safeGitFlags[flagName] {
		// Allow short flags (-v, -q, etc.) if single character
		if len(flagName) == 2 && flagName[0] == '-' && flagName[1] != '-' {
			// Single-letter short flags are generally safe
			return nil
		}

		return fmt.Errorf("unknown or unsafe Git flag: %s", flagName)
	}

	return nil
}

// SanitizePath validates a file system path.
// This prevents path traversal attacks and access to system directories.
func SanitizePath(path string) error {
	// Check for dangerous patterns
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(path) {
			return fmt.Errorf("path contains dangerous pattern: %s", path)
		}
	}

	// Check for absolute paths to system directories
	systemDirs := []string{
		"/etc/", "/usr/", "/bin/", "/sbin/", "/sys/", "/proc/",
		"C:\\Windows\\", "C:\\Program Files\\", "C:\\System32\\",
	}

	for _, sysDir := range systemDirs {
		if strings.HasPrefix(path, sysDir) {
			return fmt.Errorf("access to system directory not allowed: %s", path)
		}
	}

	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("path contains null byte")
	}

	return nil
}

// SanitizeURL validates a Git repository URL.
// This ensures the URL is in a safe format (HTTPS, SSH, or file).
func SanitizeURL(url string) error {
	if url == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	// Check for dangerous patterns
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(url) {
			return fmt.Errorf("URL contains dangerous pattern")
		}
	}

	// Validate URL scheme
	validSchemes := []string{
		"https://",
		"http://",
		"ssh://",
		"git://",
		"git@",   // SSH format (git@github.com:...)
		"file://",
		"/",      // Local path
		"./",     // Relative path
		"../",    // Relative path (though discouraged)
	}

	isValid := false
	for _, scheme := range validSchemes {
		if strings.HasPrefix(url, scheme) {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("URL has invalid or unsupported scheme: %s", url)
	}

	// Additional validation for SSH URLs
	if strings.HasPrefix(url, "git@") {
		// Format: git@host:path
		if !strings.Contains(url, ":") {
			return fmt.Errorf("invalid SSH URL format: %s", url)
		}
	}

	return nil
}

// SanitizeCommitMessage validates a commit message.
// This ensures the message doesn't contain problematic characters.
func SanitizeCommitMessage(message string) error {
	if message == "" {
		return fmt.Errorf("commit message cannot be empty")
	}

	// Check for null bytes
	if strings.Contains(message, "\x00") {
		return fmt.Errorf("commit message contains null byte")
	}

	// Check for excessively long messages (prevent DoS)
	if len(message) > 10000 {
		return fmt.Errorf("commit message too long (max 10000 characters)")
	}

	return nil
}

// SanitizeBranchName validates a Git branch name.
// This ensures the branch name follows Git conventions.
func SanitizeBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Git branch name restrictions
	invalidPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^\.`),           // Cannot start with dot
		regexp.MustCompile(`\.\.`),          // Cannot contain double dots
		regexp.MustCompile(`[~^:?*\[\]\\]`), // Cannot contain special chars
		regexp.MustCompile(`\s`),            // Cannot contain whitespace
		regexp.MustCompile(`^/|/$|//`),      // Cannot start/end with slash or have double slashes
		regexp.MustCompile(`\.lock$`),       // Cannot end with .lock
	}

	for _, pattern := range invalidPatterns {
		if pattern.MatchString(name) {
			return fmt.Errorf("branch name contains invalid pattern: %s", name)
		}
	}

	// Check length
	if len(name) > 255 {
		return fmt.Errorf("branch name too long (max 255 characters)")
	}

	return nil
}
