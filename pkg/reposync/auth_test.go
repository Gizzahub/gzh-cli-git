// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package reposync

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsSSHURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"SSH git@ format", "git@github.com:user/repo.git", true},
		{"SSH ssh:// format", "ssh://git@github.com/user/repo.git", true},
		{"SSH with port", "ssh://git@github.com:2224/user/repo.git", true},
		{"HTTPS URL", "https://github.com/user/repo.git", false},
		{"HTTP URL", "http://github.com/user/repo.git", false},
		{"HTTPS with port", "https://github.com:443/user/repo.git", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSSHURL(tt.url)
			if result != tt.expected {
				t.Errorf("isSSHURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestInjectTokenToURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		token    string
		provider string
		expected string
	}{
		{
			name:     "GitLab HTTPS",
			url:      "https://gitlab.com/group/repo.git",
			token:    "my-token",
			provider: "gitlab",
			expected: "https://oauth2:my-token@gitlab.com/group/repo.git",
		},
		{
			name:     "GitHub HTTPS",
			url:      "https://github.com/user/repo.git",
			token:    "ghp_xxxx",
			provider: "github",
			expected: "https://x-access-token:ghp_xxxx@github.com/user/repo.git",
		},
		{
			name:     "Gitea HTTPS",
			url:      "https://gitea.example.com/org/repo.git",
			token:    "gitea-token",
			provider: "gitea",
			expected: "https://gitea-token@gitea.example.com/org/repo.git",
		},
		{
			name:     "Unknown provider defaults to oauth2",
			url:      "https://unknown.com/repo.git",
			token:    "token",
			provider: "unknown",
			expected: "https://oauth2:token@unknown.com/repo.git",
		},
		{
			name:     "SSH URL unchanged",
			url:      "git@github.com:user/repo.git",
			token:    "token",
			provider: "github",
			expected: "git@github.com:user/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := injectTokenToURL(tt.url, tt.token, tt.provider)
			if err != nil {
				t.Fatalf("injectTokenToURL() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("injectTokenToURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPrepareAuth_HTTPS_WithToken(t *testing.T) {
	auth := AuthConfig{
		Token:    "test-token",
		Provider: "gitlab",
	}

	result, err := PrepareAuth("https://gitlab.com/group/repo.git", auth)
	if err != nil {
		t.Fatalf("PrepareAuth() error = %v", err)
	}

	expected := "https://oauth2:test-token@gitlab.com/group/repo.git"
	if result.CloneURL != expected {
		t.Errorf("CloneURL = %q, want %q", result.CloneURL, expected)
	}

	if len(result.Env) != 0 {
		t.Errorf("Env should be empty for HTTPS, got %v", result.Env)
	}
}

func TestPrepareAuth_HTTPS_NoToken(t *testing.T) {
	auth := AuthConfig{
		Provider: "gitlab",
		// No token - should fallback to system
	}

	result, err := PrepareAuth("https://gitlab.com/group/repo.git", auth)
	if err != nil {
		t.Fatalf("PrepareAuth() error = %v", err)
	}

	// URL should be unchanged (fallback to system credential helper)
	expected := "https://gitlab.com/group/repo.git"
	if result.CloneURL != expected {
		t.Errorf("CloneURL = %q, want %q (unchanged)", result.CloneURL, expected)
	}
}

func TestPrepareAuth_SSH_WithKeyPath(t *testing.T) {
	// Create a temporary SSH key file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key")
	if err := os.WriteFile(keyPath, []byte("fake-ssh-key\n"), 0o600); err != nil {
		t.Fatalf("Failed to create test key file: %v", err)
	}

	auth := AuthConfig{
		SSHKeyPath: keyPath,
		SSHPort:    2224,
	}

	result, err := PrepareAuth("git@gitlab.com:group/repo.git", auth)
	if err != nil {
		t.Fatalf("PrepareAuth() error = %v", err)
	}

	// URL should be unchanged for SSH
	if result.CloneURL != "git@gitlab.com:group/repo.git" {
		t.Errorf("CloneURL should be unchanged for SSH")
	}

	// Should have GIT_SSH_COMMAND env var
	if len(result.Env) != 1 {
		t.Fatalf("Expected 1 env var, got %d", len(result.Env))
	}

	if !strings.HasPrefix(result.Env[0], "GIT_SSH_COMMAND=") {
		t.Errorf("Expected GIT_SSH_COMMAND, got %s", result.Env[0])
	}

	// Should contain key path and port
	if !strings.Contains(result.Env[0], keyPath) {
		t.Errorf("GIT_SSH_COMMAND should contain key path")
	}
	if !strings.Contains(result.Env[0], "-p 2224") {
		t.Errorf("GIT_SSH_COMMAND should contain custom port")
	}
}

func TestPrepareAuth_SSH_WithKeyContent(t *testing.T) {
	auth := AuthConfig{
		SSHKeyContent: "-----BEGIN OPENSSH PRIVATE KEY-----\nfake-key-content\n-----END OPENSSH PRIVATE KEY-----",
	}

	result, err := PrepareAuth("git@gitlab.com:group/repo.git", auth)
	if err != nil {
		t.Fatalf("PrepareAuth() error = %v", err)
	}

	// Should have created a temp file
	if result.TempKeyPath == "" {
		t.Error("TempKeyPath should be set when using SSHKeyContent")
	}

	// Should have warning about temp file
	if len(result.Warnings) == 0 {
		t.Error("Expected warning about temp key file")
	}

	// Verify temp file exists and has correct permissions
	info, err := os.Stat(result.TempKeyPath)
	if err != nil {
		t.Fatalf("Temp key file not found: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("Temp key file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Cleanup
	os.Remove(result.TempKeyPath)
}

func TestPrepareAuth_SSH_NoKey(t *testing.T) {
	auth := AuthConfig{
		// No SSH key - should fallback to system
	}

	result, err := PrepareAuth("git@gitlab.com:group/repo.git", auth)
	if err != nil {
		t.Fatalf("PrepareAuth() error = %v", err)
	}

	// URL should be unchanged
	if result.CloneURL != "git@gitlab.com:group/repo.git" {
		t.Errorf("CloneURL should be unchanged")
	}

	// No env vars (fallback to system SSH)
	if len(result.Env) != 0 {
		t.Errorf("Env should be empty for fallback, got %v", result.Env)
	}
}

func TestPrepareAuth_SSH_KeyNotFound(t *testing.T) {
	auth := AuthConfig{
		SSHKeyPath: "/nonexistent/path/to/key",
	}

	_, err := PrepareAuth("git@gitlab.com:group/repo.git", auth)
	if err == nil {
		t.Error("Expected error for nonexistent key file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error should mention file not found, got: %v", err)
	}
}

func TestBuildSSHCommand(t *testing.T) {
	tests := []struct {
		name     string
		keyPath  string
		sshPort  int
		contains []string
	}{
		{
			name:     "Default port",
			keyPath:  "/path/to/key",
			sshPort:  0,
			contains: []string{"ssh", "-i '/path/to/key'", "IdentitiesOnly=yes"},
		},
		{
			name:     "Custom port",
			keyPath:  "/path/to/key",
			sshPort:  2224,
			contains: []string{"-p 2224"},
		},
		{
			name:     "Standard port 22 omitted",
			keyPath:  "/path/to/key",
			sshPort:  22,
			contains: []string{"ssh", "-i '/path/to/key'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildSSHCommand(tt.keyPath, tt.sshPort)
			for _, c := range tt.contains {
				if !strings.Contains(result, c) {
					t.Errorf("buildSSHCommand() = %q, should contain %q", result, c)
				}
			}
		})
	}
}

func TestBuildSSHCommandProtectsKeyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell to verify GIT_SSH_COMMAND parsing")
	}

	tempDir := t.TempDir()
	marker := filepath.Join(tempDir, "injected")
	keyPath := filepath.Join(tempDir, "key path;touch "+marker+" '$(printf hacked)'")
	capture := filepath.Join(tempDir, "ssh-args")
	fakeSSH := filepath.Join(tempDir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_FILE\"\n"), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}

	command := buildSSHCommand(keyPath, 2224)
	cmd := exec.CommandContext(context.Background(), "sh", "-c", command+" git@example.com:group/repo.git")
	cmd.Env = append(os.Environ(), "PATH="+tempDir+":/usr/bin:/bin", "CAPTURE_FILE="+capture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run quoted SSH command: %v, output: %s", err, output)
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("key path was interpreted as shell syntax; marker stat error: %v", err)
	}
	args, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read fake ssh arguments: %v", err)
	}
	if !strings.Contains(string(args), keyPath) {
		t.Errorf("fake ssh did not receive the complete key path; args=%q", args)
	}
}

func TestMaskTokenInURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "GitLab style",
			url:      "https://oauth2:secret-token@gitlab.com/group/repo.git",
			expected: "https://oauth2:***@gitlab.com/group/repo.git",
		},
		{
			name:     "Gitea style (token only)",
			url:      "https://secret-token@gitea.com/repo.git",
			expected: "https://***@gitea.com/repo.git",
		},
		{
			name:     "No credentials",
			url:      "https://github.com/user/repo.git",
			expected: "https://github.com/user/repo.git",
		},
		{
			name:     "SSH URL unchanged",
			url:      "git@github.com:user/repo.git",
			expected: "git@github.com:user/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskTokenInURL(tt.url)
			if result != tt.expected {
				t.Errorf("MaskTokenInURL(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

func TestExpandHomePath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Home path",
			path:     "~/.ssh/id_rsa",
			expected: filepath.Join(home, ".ssh", "id_rsa"),
		},
		{
			name:     "Absolute path unchanged",
			path:     "/etc/ssh/key",
			expected: "/etc/ssh/key",
		},
		{
			name:     "Relative path unchanged",
			path:     "./key",
			expected: "./key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandHomePath(tt.path)
			if err != nil {
				t.Fatalf("expandHomePath() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("expandHomePath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}
