package gitcmd

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
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
func SanitizeURL(value string) error {
	if value == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	// URLs are passed as one argv value, not through a shell. Shell
	// metacharacters (notably '&' in query strings) are therefore valid URL
	// data. Reject only characters that can alter an argv value and values that
	// Git could interpret as an option.
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("URL cannot start with a dash")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return fmt.Errorf("URL contains control or whitespace character")
		}
	}

	// SCP-style SSH URLs are not understood by net/url. Accept any non-empty
	// username/host pair, rather than hard-coding the conventional "git" user.
	if looksLikeSCPURL(value) {
		return nil
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme == "" {
		// Git accepts local absolute and relative paths as remotes, including
		// paths outside the current directory such as ../shared/repo.git.
		if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
			return nil
		}
		return fmt.Errorf("URL has invalid or unsupported scheme: %s", value)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "ssh" && parsed.Scheme != "git" && parsed.Scheme != "file" {
		return fmt.Errorf("URL has invalid or unsupported scheme: %s", value)
	}
	if parsed.Scheme != "file" && parsed.Host == "" {
		return fmt.Errorf("URL has no host: %s", value)
	}

	return nil
}

func looksLikeSCPURL(value string) bool {
	colon := strings.IndexByte(value, ':')
	if colon <= 0 || strings.Contains(value[:colon], "/") {
		return false
	}
	left := value[:colon]
	at := strings.LastIndexByte(left, '@')
	return at > 0 && at < len(left)-1 && colon < len(value)-1
}

// SanitizeRemoteName validates a remote name before it is passed to git.
// Remote names are configuration keys, not arbitrary command arguments; a
// deliberately narrow character set also prevents option injection and
// malformed ref namespaces.
func SanitizeRemoteName(name string) error {
	if err := validateGitRefName(name, "remote name"); err != nil {
		return err
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("remote name cannot start with a dash")
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
	if err := validateGitRefName(name, "branch name"); err != nil {
		return err
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("branch name cannot start with a dash")
	}
	return nil
}

// validateGitRefName mirrors git check-ref-format's safety rules for a single
// ref name. It intentionally permits punctuation such as ';', '&', '$', and
// '`': callers pass refs as argv values and these are valid Git ref bytes.
func validateGitRefName(name, kind string) error {
	if name == "" {
		return fmt.Errorf("%s cannot be empty", kind)
	}
	if len(name) > 255 {
		return fmt.Errorf("%s too long (max 255 characters)", kind)
	}
	if name == "@" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "//") {
		return fmt.Errorf("%s contains invalid slash or ref sequence", kind)
	}
	if strings.Contains(name, "..") || strings.Contains(name, "@{") {
		return fmt.Errorf("%s contains invalid ref sequence", kind)
	}

	for _, r := range name {
		if unicode.IsControl(r) || unicode.IsSpace(r) || strings.ContainsRune("~^:?*[\\", r) {
			return fmt.Errorf("%s contains invalid character %q", kind, r)
		}
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".") || strings.HasSuffix(component, ".lock") {
			return fmt.Errorf("%s contains invalid path component %q", kind, component)
		}
	}
	return nil
}
