package gitcmd

import (
	"fmt"
	"regexp"
	"strings"
)

// Dangerous patterns that could enable command injection or path traversal.
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[;&|><$]`),                // Command separators and redirections
	regexp.MustCompile(`\$\(`),                    // Command substitution $(...)
	regexp.MustCompile("`"),                       // Backtick command substitution
	regexp.MustCompile(`\.\./`),                   // Path traversal (relative)
	regexp.MustCompile(`^/(?:etc|usr|bin|sbin)/`), // System directories
	regexp.MustCompile(`\x00`),                    // Null bytes
	regexp.MustCompile(`\r|\n`),                   // Newlines (could break parsing)
}

// SanitizeArgs validates and sanitizes Git command arguments.
//
// Git is executed via exec.CommandContext without a shell, so a flag allowlist
// provides no shell-injection defense — it only blocks legitimate flags until
// each one is manually added. The residual threat is option injection (a
// user-supplied value parsed by git as a flag), which is handled by the '--'
// end-of-options separator and the per-value validators (SanitizeBranchName,
// SanitizePath, SanitizeURL, SanitizeCommitMessage) at the call sites.
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

		// Trim and add to sanitized list
		sanitized = append(sanitized, strings.TrimSpace(arg))
	}

	return sanitized, nil
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
		"git@", // SSH format (git@github.com:...)
		"file://",
		"/",   // Local path
		"./",  // Relative path
		"../", // Relative path (though discouraged)
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

// SanitizeRemoteName validates a remote name before it is passed to git.
// Remote names are configuration keys, not arbitrary command arguments; a
// deliberately narrow character set also prevents option injection and
// malformed ref namespaces.
func SanitizeRemoteName(name string) error {
	if name == "" {
		return fmt.Errorf("remote name cannot be empty")
	}
	if len(name) > 255 {
		return fmt.Errorf("remote name too long")
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			if i == 0 && r == '.' {
				return fmt.Errorf("remote name cannot start with a dot")
			}
			continue
		}
		return fmt.Errorf("remote name contains unsupported character %q", r)
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") || strings.Contains(name, "..") {
		return fmt.Errorf("remote name has invalid suffix or ref sequence")
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
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(name) {
			return fmt.Errorf("branch name contains dangerous pattern")
		}
	}

	// Git branch name restrictions
	invalidPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^-`),            // Cannot start with dash (option injection)
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
