package gitcmd

import (
	"strings"
	"testing"
)

// TestSanitizeArgs tests the argument sanitization function.
func TestSanitizeArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{
			name:    "empty args",
			args:    []string{},
			want:    []string{},
			wantErr: false,
		},
		{
			name:    "safe args",
			args:    []string{"status", "--porcelain"},
			want:    []string{"status", "--porcelain"},
			wantErr: false,
		},
		{
			name:    "command injection with semicolon",
			args:    []string{"status", "; rm -rf /"},
			wantErr: true,
		},
		{
			name:    "command injection with pipe",
			args:    []string{"log", "| cat /etc/passwd"},
			wantErr: true,
		},
		{
			name:    "command injection with ampersand",
			args:    []string{"clone", "url && malicious"},
			wantErr: true,
		},
		{
			name:    "command substitution with dollar paren",
			args:    []string{"log", "$(whoami)"},
			wantErr: true,
		},
		{
			name:    "command substitution with backticks",
			args:    []string{"log", "`whoami`"},
			wantErr: true,
		},
		{
			name:    "path traversal",
			args:    []string{"status", "../../../etc/passwd"},
			wantErr: true,
		},
		{
			name:    "system directory access",
			args:    []string{"add", "/etc/hosts"},
			wantErr: true,
		},
		{
			name:    "null byte injection",
			args:    []string{"log", "file\x00.txt"},
			wantErr: true,
		},
		{
			name:    "newline injection",
			args:    []string{"commit", "message\nmalicious"},
			wantErr: true,
		},
		{
			name:    "safe flag with value",
			args:    []string{"clone", "--branch=main", "url"},
			want:    []string{"clone", "--branch=main", "url"},
			wantErr: false,
		},
		{
			name:    "short flag",
			args:    []string{"status", "-v"},
			want:    []string{"status", "-v"},
			wantErr: false,
		},
		{
			// Regression: these flags were rejected by the removed whitelist,
			// which made `gz-git stash save -u` always fail.
			name:    "stash flags previously blocked by whitelist",
			args:    []string{"stash", "push", "--include-untracked", "--keep-index"},
			want:    []string{"stash", "push", "--include-untracked", "--keep-index"},
			wantErr: false,
		},
		{
			// No shell is involved, so a non-allowlisted flag without dangerous
			// characters is accepted; option injection is handled by '--' + value validators.
			name:    "non-allowlisted flag is accepted (no shell)",
			args:    []string{"log", "--first-parent"},
			want:    []string{"log", "--first-parent"},
			wantErr: false,
		},
		{
			// The '--' end-of-options separator must pass through untouched.
			name:    "end-of-options separator",
			args:    []string{"checkout", "--", "path/to/file"},
			want:    []string{"checkout", "--", "path/to/file"},
			wantErr: false,
		},
		{
			name:    "arguments with whitespace trimming",
			args:    []string{"  status  ", "  --porcelain  "},
			want:    []string{"status", "--porcelain"},
			wantErr: false,
		},
		{
			name:    "redirection attempt",
			args:    []string{"log", "> /tmp/output"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeArgs(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SanitizeArgs() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("SanitizeArgs() unexpected error: %v", err)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("SanitizeArgs() got %d args, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SanitizeArgs() arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestSanitizePath tests path sanitization.
func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "safe relative path",
			path:    "./myrepo",
			wantErr: false,
		},
		{
			name:    "safe absolute path",
			path:    "/home/user/repos/myrepo",
			wantErr: false,
		},
		{
			name:    "path traversal",
			path:    "../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "system directory /etc",
			path:    "/etc/hosts",
			wantErr: true,
		},
		{
			name:    "system directory /usr",
			path:    "/usr/bin/git",
			wantErr: true,
		},
		{
			name:    "system directory /bin",
			path:    "/bin/sh",
			wantErr: true,
		},
		{
			name:    "windows system directory",
			path:    "C:\\Windows\\System32",
			wantErr: true,
		},
		{
			name:    "null byte in path",
			path:    "/home/user\x00/repo",
			wantErr: true,
		},
		{
			name:    "command injection in path",
			path:    "/home/user; rm -rf /",
			wantErr: true,
		},
		{
			name:    "pipe in path",
			path:    "/home/user | cat",
			wantErr: true,
		},
		{
			name:    "newline in path",
			path:    "/home/user\n/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SanitizePath(tt.path)

			if tt.wantErr && err == nil {
				t.Errorf("SanitizePath() expected error for %q, got nil", tt.path)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("SanitizePath() unexpected error for %q: %v", tt.path, err)
			}
		})
	}
}

// TestSanitizeURL tests URL sanitization.
func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "https URL",
			url:     "https://github.com/user/repo.git",
			wantErr: false,
		},
		{
			name:    "http URL",
			url:     "http://github.com/user/repo.git",
			wantErr: false,
		},
		{
			name:    "ssh URL",
			url:     "ssh://git@github.com/user/repo.git",
			wantErr: false,
		},
		{
			name:    "git protocol",
			url:     "git://github.com/user/repo.git",
			wantErr: false,
		},
		{
			name:    "ssh shorthand",
			url:     "git@github.com:user/repo.git",
			wantErr: false,
		},
		{
			name:    "file protocol",
			url:     "file:///home/user/repo",
			wantErr: false,
		},
		{
			name:    "local path",
			url:     "/home/user/repo",
			wantErr: false,
		},
		{
			name:    "relative path",
			url:     "./repo",
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid scheme",
			url:     "ftp://example.com/repo",
			wantErr: true,
		},
		{
			name:    "command injection in URL",
			url:     "https://github.com/user/repo.git; rm -rf /",
			wantErr: true,
		},
		{
			name:    "invalid ssh format",
			url:     "git@github.com",
			wantErr: true,
		},
		{
			name:    "null byte in URL",
			url:     "https://github.com\x00/user/repo.git",
			wantErr: true,
		},
		{
			name:    "pipe in URL",
			url:     "https://github.com | cat",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SanitizeURL(tt.url)

			if tt.wantErr && err == nil {
				t.Errorf("SanitizeURL() expected error for %q, got nil", tt.url)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("SanitizeURL() unexpected error for %q: %v", tt.url, err)
			}
		})
	}
}

// TestSanitizeCommitMessage tests commit message sanitization.
func TestSanitizeCommitMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantErr bool
	}{
		{
			name:    "valid commit message",
			message: "feat: add new feature",
			wantErr: false,
		},
		{
			name:    "multiline commit message",
			message: "feat: add new feature\n\nThis is a detailed description.",
			wantErr: false,
		},
		{
			name:    "empty message",
			message: "",
			wantErr: true,
		},
		{
			name:    "null byte in message",
			message: "feat: add feature\x00",
			wantErr: true,
		},
		{
			name:    "excessively long message",
			message: strings.Repeat("a", 10001),
			wantErr: true,
		},
		{
			name:    "message with special characters",
			message: "fix: resolve issue #123 (urgent!)",
			wantErr: false,
		},
		{
			name:    "message with emojis",
			message: "feat: ✨ add new feature",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SanitizeCommitMessage(tt.message)

			if tt.wantErr && err == nil {
				t.Errorf("SanitizeCommitMessage() expected error for message length %d, got nil", len(tt.message))
			}

			if !tt.wantErr && err != nil {
				t.Errorf("SanitizeCommitMessage() unexpected error: %v", err)
			}
		})
	}
}

// TestSanitizeBranchName tests branch name sanitization.
func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		wantErr    bool
	}{
		{
			name:       "valid branch name",
			branchName: "feature/new-feature",
			wantErr:    false,
		},
		{
			name:       "main branch",
			branchName: "main",
			wantErr:    false,
		},
		{
			name:       "develop branch",
			branchName: "develop",
			wantErr:    false,
		},
		{
			name:       "branch with numbers",
			branchName: "feature/issue-123",
			wantErr:    false,
		},
		{
			name:       "branch with hyphens",
			branchName: "feature/my-new-feature",
			wantErr:    false,
		},
		{
			name:       "empty branch name",
			branchName: "",
			wantErr:    true,
		},
		{
			name:       "branch starting with dot",
			branchName: ".feature",
			wantErr:    true,
		},
		{
			name:       "branch starting with dash (option injection)",
			branchName: "--upload-pack=evil",
			wantErr:    true,
		},
		{
			name:       "branch with double dots",
			branchName: "feature..branch",
			wantErr:    true,
		},
		{
			name:       "branch with tilde",
			branchName: "feature~1",
			wantErr:    true,
		},
		{
			name:       "branch with caret",
			branchName: "feature^",
			wantErr:    true,
		},
		{
			name:       "branch with colon",
			branchName: "feature:branch",
			wantErr:    true,
		},
		{
			name:       "branch with question mark",
			branchName: "feature?",
			wantErr:    true,
		},
		{
			name:       "branch with asterisk",
			branchName: "feature*",
			wantErr:    true,
		},
		{
			name:       "branch with whitespace",
			branchName: "feature branch",
			wantErr:    true,
		},
		{
			name:       "branch starting with slash",
			branchName: "/feature",
			wantErr:    true,
		},
		{
			name:       "branch ending with slash",
			branchName: "feature/",
			wantErr:    true,
		},
		{
			name:       "branch with double slashes",
			branchName: "feature//branch",
			wantErr:    true,
		},
		{
			name:       "branch ending with .lock",
			branchName: "feature.lock",
			wantErr:    true,
		},
		{
			name:       "excessively long branch name",
			branchName: strings.Repeat("a", 256),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SanitizeBranchName(tt.branchName)

			if tt.wantErr && err == nil {
				t.Errorf("SanitizeBranchName() expected error for %q, got nil", tt.branchName)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("SanitizeBranchName() unexpected error for %q: %v", tt.branchName, err)
			}
		})
	}
}

// TestDangerousPatternsComprehensive tests all dangerous patterns.
func TestDangerousPatternsComprehensive(t *testing.T) {
	dangerousInputs := []string{
		"; echo malicious",
		"& whoami",
		"| cat /etc/passwd",
		"> /tmp/output",
		"< /etc/passwd",
		"$(malicious)",
		"`malicious`",
		"../../../etc/passwd",
		"/etc/shadow",
		"test\x00file",
		"test\nfile",
		"test\rfile",
	}

	for _, input := range dangerousInputs {
		t.Run("dangerous_input_"+input[:min(len(input), 20)], func(t *testing.T) {
			// Should fail in SanitizeArgs
			_, err := SanitizeArgs([]string{input})
			if err == nil {
				t.Errorf("SanitizeArgs() should reject dangerous input: %q", input)
			}
		})
	}
}
