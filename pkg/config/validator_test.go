// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"testing"
)

func TestValidateProfile(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		profile *Profile
		wantErr bool
	}{
		{
			name: "valid profile",
			profile: &Profile{
				Name:       "work",
				Provider:   "gitlab",
				BaseURL:    "https://gitlab.com",
				CloneProto: "ssh",
				SSHPort:    2224,
				Parallel:   10,
			},
			wantErr: false,
		},
		{
			name:    "nil profile",
			profile: nil,
			wantErr: true,
		},
		{
			name: "empty name",
			profile: &Profile{
				Name: "",
			},
			wantErr: true,
		},
		{
			name: "invalid name",
			profile: &Profile{
				Name: "invalid name!", // Contains invalid characters
			},
			wantErr: true,
		},
		{
			name: "invalid provider",
			profile: &Profile{
				Name:     "test",
				Provider: "bitbucket", // Not supported
			},
			wantErr: true,
		},
		{
			name: "invalid clone protocol",
			profile: &Profile{
				Name:       "test",
				CloneProto: "git", // Not supported
			},
			wantErr: true,
		},
		{
			name: "invalid SSH port",
			profile: &Profile{
				Name:    "test",
				SSHPort: 99999, // Out of range
			},
			wantErr: true,
		},
		{
			name: "negative parallel count",
			profile: &Profile{
				Name:     "test",
				Parallel: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid subgroup mode",
			profile: &Profile{
				Name:         "test",
				SubgroupMode: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateProfile(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProfile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSyncConfig(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		config  *SyncConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false, // nil is valid
		},
		{
			name: "valid config",
			config: &SyncConfig{
				Strategy:   "pull",
				MaxRetries: 3,
			},
			wantErr: false,
		},
		{
			name: "invalid strategy",
			config: &SyncConfig{
				Strategy: "invalid",
			},
			wantErr: true,
		},
		{
			name: "negative max retries",
			config: &SyncConfig{
				MaxRetries: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateSyncConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSyncConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpandEnvVarsInProfile(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_TOKEN", "secret123")
	os.Setenv("TEST_URL", "https://test.com")
	defer os.Unsetenv("TEST_TOKEN")
	defer os.Unsetenv("TEST_URL")

	validator := NewValidator()

	tests := []struct {
		name     string
		profile  *Profile
		expected *Profile
	}{
		{
			name: "expand token",
			profile: &Profile{
				Name:  "test",
				Token: "${TEST_TOKEN}",
			},
			expected: &Profile{
				Name:  "test",
				Token: "secret123",
			},
		},
		{
			name: "expand base URL",
			profile: &Profile{
				Name:    "test",
				BaseURL: "${TEST_URL}",
			},
			expected: &Profile{
				Name:    "test",
				BaseURL: "https://test.com",
			},
		},
		{
			name: "no expansion needed",
			profile: &Profile{
				Name:  "test",
				Token: "plain-text-token",
			},
			expected: &Profile{
				Name:  "test",
				Token: "plain-text-token",
			},
		},
		{
			name: "missing env var",
			profile: &Profile{
				Name:  "test",
				Token: "${MISSING_VAR}",
			},
			expected: &Profile{
				Name:  "test",
				Token: "", // Empty string for missing var
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ExpandEnvVarsInProfile(tt.profile)
			if err != nil {
				t.Errorf("ExpandEnvVarsInProfile() error = %v", err)
				return
			}

			if tt.profile.Token != tt.expected.Token {
				t.Errorf("Token = %v, want %v", tt.profile.Token, tt.expected.Token)
			}
			if tt.profile.BaseURL != tt.expected.BaseURL {
				t.Errorf("BaseURL = %v, want %v", tt.profile.BaseURL, tt.expected.BaseURL)
			}
		})
	}
}

func TestIsValidProfileName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"alphanumeric", "work", true},
		{"with dash", "my-work", true},
		{"with underscore", "my_work", true},
		{"with number", "work2", true},
		{"empty", "", false},
		{"with space", "my work", false},
		{"with special char", "work!", false},
		{"with dot", "work.profile", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidProfileName(tt.input)
			if got != tt.want {
				t.Errorf("IsValidProfileName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidProvider(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"github", "github", true},
		{"gitlab", "gitlab", true},
		{"gitea", "gitea", true},
		{"bitbucket", "bitbucket", false},
		{"invalid", "invalid", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidProvider(tt.input)
			if got != tt.want {
				t.Errorf("IsValidProvider(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"plain token", "abc123", "abc123"}, // Plain tokens are not sanitized by this function
		{"url with credentials", "https://user:pass@example.com", "https://***:***@example.com"},
		{"url with token param", "https://api.com?token=secret123", "https://api.com?token=***"},
		{"url with token in query", "https://api.com?foo=bar&token=secret", "https://api.com?token=***"}, // Query params before token are removed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeToken(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeToken(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeProvider(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase", "github", "github"},
		{"uppercase", "GITHUB", "github"},
		{"mixed case", "GitHub", "github"},
		{"with spaces", " gitlab ", "gitlab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeProvider(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeProvider(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateConfig_ChildConfigMode(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config is valid",
			config:  nil,
			wantErr: false,
		},
		{
			name: "empty childConfigMode is valid",
			config: &Config{
				ChildConfigMode: "",
			},
			wantErr: false,
		},
		{
			name: "repositories mode is valid",
			config: &Config{
				ChildConfigMode: ChildConfigModeRepositories,
			},
			wantErr: false,
		},
		{
			name: "workspaces mode is valid",
			config: &Config{
				ChildConfigMode: ChildConfigModeWorkspaces,
			},
			wantErr: false,
		},
		{
			name: "none mode is valid",
			config: &Config{
				ChildConfigMode: ChildConfigModeNone,
			},
			wantErr: false,
		},
		{
			name: "invalid mode returns error",
			config: &Config{
				ChildConfigMode: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsValidBaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"empty is allowed (optional field)", "", true},
		{"whitespace only collapses to empty", "   ", true},
		{"https is valid", "https://github.example.com", true},
		{"http is valid", "http://ghe.insecure.local", true},
		{"https with path is valid", "https://github.example.com/api/v3", true},
		{"https with port is valid", "https://ghe.io:8443", true},
		{"https without host rejected", "https://", false},
		{"http without host rejected", "http://", false},
		{"ftp scheme rejected", "ftp://github.example.com", false},
		{"ssh scheme rejected", "ssh://git@github.example.com", false},
		{"missing scheme rejected", "github.example.com", false},
		{"javascript scheme rejected", "javascript:alert(1)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidBaseURL(tt.url); got != tt.want {
				t.Errorf("IsValidBaseURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestValidateProfile_InvalidBaseURL(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{"empty base URL is allowed", "", false},
		{"https base URL is allowed", "https://github.example.com", false},
		{"ftp base URL rejected", "ftp://github.example.com", true},
		{"missing scheme rejected", "github.example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Profile{Name: "test", Provider: "github", BaseURL: tt.baseURL}
			err := validator.ValidateProfile(p)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProfile() baseURL=%q error = %v, wantErr %v", tt.baseURL, err, tt.wantErr)
			}
		})
	}
}

func TestValidateForgeSource_InvalidBaseURL(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{"empty base URL is allowed", "", false},
		{"https base URL is allowed", "https://github.example.com", false},
		{"ftp base URL rejected", "ftp://x.example.com", true},
		{"missing scheme rejected", "example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ForgeSource{Provider: "github", Org: "myorg", BaseURL: tt.baseURL}
			err := validator.ValidateForgeSource(s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateForgeSource() baseURL=%q error = %v, wantErr %v", tt.baseURL, err, tt.wantErr)
			}
		})
	}
}
