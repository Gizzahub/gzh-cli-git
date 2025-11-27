package commit

import (
	"context"
	"strings"
	"testing"
)

func TestValidator_Validate(t *testing.T) {
	ctx := context.Background()
	v := NewValidator()

	// Create test template
	tmpl := &Template{
		Name:   "test",
		Format: "{{.Type}}: {{.Message}}",
		Rules: []ValidationRule{
			{
				Type:    "pattern",
				Pattern: "^(feat|fix|docs): .+",
				Message: "must start with feat, fix, or docs",
			},
			{
				Type:    "length",
				Pattern: "^.{1,50}$",
				Message: "subject must be 1-50 characters",
			},
		},
	}

	tests := []struct {
		name       string
		message    string
		wantValid  bool
		wantErrors int
	}{
		{
			name:       "valid message",
			message:    "feat: add new feature",
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name:       "valid fix message",
			message:    "fix: correct bug",
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name:       "invalid type",
			message:    "invalid: wrong type",
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name:       "too long",
			message:    "feat: this is a very long commit message that exceeds the maximum allowed length",
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name:       "empty message",
			message:    "",
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name:       "multiple errors",
			message:    "toolong: this is a very long commit message that exceeds the maximum allowed length",
			wantValid:  false,
			wantErrors: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.Validate(ctx, tt.message, tmpl)
			if err != nil {
				t.Fatalf("Validate() error: %v", err)
			}
			if result.Valid != tt.wantValid {
				t.Errorf("Validate() valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Validate() errors = %d, want %d", len(result.Errors), tt.wantErrors)
				for _, e := range result.Errors {
					t.Logf("  Error: %s", e.Message)
				}
			}
		})
	}
}

func TestValidator_ValidateNilTemplate(t *testing.T) {
	ctx := context.Background()
	v := NewValidator()

	result, err := v.Validate(ctx, "test message", nil)
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
	if result.Valid {
		t.Error("Validate() with nil template should be invalid")
	}
	if len(result.Errors) == 0 {
		t.Error("Validate() with nil template should have errors")
	}
}

func TestValidator_ValidateConventionalCommits(t *testing.T) {
	ctx := context.Background()
	mgr := NewTemplateManager()
	tmpl, err := mgr.Load(ctx, "conventional")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	v := NewValidator()

	tests := []struct {
		name       string
		message    string
		wantValid  bool
		wantErrors int
	}{
		{
			name:       "valid feat",
			message:    "feat: add new feature",
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name:       "valid feat with scope",
			message:    "feat(api): add new endpoint",
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name:       "valid fix",
			message:    "fix(parser): handle null values",
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name:       "valid docs",
			message:    "docs: update README",
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name:       "valid with body",
			message:    "feat: add feature\n\nThis is the body.",
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name:       "invalid type",
			message:    "invalid: wrong type",
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name:       "missing colon",
			message:    "feat add feature",
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name:       "too long subject",
			message:    "feat: " + strings.Repeat("x", 70),
			wantValid:  false,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.Validate(ctx, tt.message, tmpl)
			if err != nil {
				t.Fatalf("Validate() error: %v", err)
			}
			if result.Valid != tt.wantValid {
				t.Errorf("Validate() valid = %v, want %v", result.Valid, tt.wantValid)
				for _, e := range result.Errors {
					t.Logf("  Error: %s", e.Message)
				}
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Validate() errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestValidator_ValidateInteractive(t *testing.T) {
	ctx := context.Background()
	v := NewValidator()

	tests := []struct {
		name      string
		message   string
		wantValid bool
	}{
		{
			name:      "valid conventional commit",
			message:   "feat: add feature",
			wantValid: true,
		},
		{
			name:      "invalid conventional commit",
			message:   "invalid: wrong",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.ValidateInteractive(ctx, tt.message)
			if err != nil {
				t.Fatalf("ValidateInteractive() error: %v", err)
			}
			if result.Valid != tt.wantValid {
				t.Errorf("ValidateInteractive() valid = %v, want %v", result.Valid, tt.wantValid)
			}
		})
	}
}

func TestValidator_Warnings(t *testing.T) {
	ctx := context.Background()
	v := NewValidator()

	tmpl := &Template{
		Name:   "test",
		Format: "{{.Message}}",
		Rules:  []ValidationRule{},
	}

	tests := []struct {
		name         string
		message      string
		wantWarnings int
		checkWarning func([]ValidationWarning) bool
	}{
		{
			name:         "period at end",
			message:      "add feature.",
			wantWarnings: 1,
			checkWarning: func(warnings []ValidationWarning) bool {
				for _, w := range warnings {
					if strings.Contains(w.Message, "period") {
						return true
					}
				}
				return false
			},
		},
		{
			name:         "past tense",
			message:      "added feature",
			wantWarnings: 1,
			checkWarning: func(warnings []ValidationWarning) bool {
				for _, w := range warnings {
					if strings.Contains(w.Message, "imperative") {
						return true
					}
				}
				return false
			},
		},
		{
			name:         "uppercase after colon",
			message:      "feat: Add feature",
			wantWarnings: 1,
			checkWarning: func(warnings []ValidationWarning) bool {
				for _, w := range warnings {
					if strings.Contains(w.Message, "uppercase") {
						return true
					}
				}
				return false
			},
		},
		{
			name:         "very long body line",
			message:      "feat: add feature\n\n" + strings.Repeat("x", 120),
			wantWarnings: 1,
			checkWarning: func(warnings []ValidationWarning) bool {
				for _, w := range warnings {
					if strings.Contains(w.Message, "long") {
						return true
					}
				}
				return false
			},
		},
		{
			name:         "proper imperative",
			message:      "add feature",
			wantWarnings: 0,
			checkWarning: func(warnings []ValidationWarning) bool {
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.Validate(ctx, tt.message, tmpl)
			if err != nil {
				t.Fatalf("Validate() error: %v", err)
			}
			if len(result.Warnings) < tt.wantWarnings {
				t.Errorf("Validate() warnings = %d, want at least %d", len(result.Warnings), tt.wantWarnings)
			}
			if !tt.checkWarning(result.Warnings) {
				t.Errorf("Validate() expected specific warning not found")
				for _, w := range result.Warnings {
					t.Logf("  Warning: %s", w.Message)
				}
			}
		})
	}
}

func TestValidator_ImperativeMood(t *testing.T) {
	v := &validator{}

	tests := []struct {
		subject string
		want    bool
	}{
		{"add feature", true},
		{"fix bug", true},
		{"update docs", true},
		{"added feature", false},
		{"fixed bug", false},
		{"updating docs", false},
		{"feat: add feature", true},
		{"feat: added feature", false},
		{"need this change", true}, // exception
		{"feed data", true},        // exception
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			got := v.isImperativeMood(tt.subject)
			if got != tt.want {
				t.Errorf("isImperativeMood(%q) = %v, want %v", tt.subject, got, tt.want)
			}
		})
	}
}

func TestFormatErrors(t *testing.T) {
	tests := []struct {
		name   string
		result *ValidationResult
		want   string
	}{
		{
			name: "valid result",
			result: &ValidationResult{
				Valid:  true,
				Errors: []ValidationError{},
			},
			want: "",
		},
		{
			name: "single error",
			result: &ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Message: "test error"},
				},
			},
			want: "Validation failed:\n  test error\n",
		},
		{
			name: "error with line",
			result: &ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Message: "test error", Line: 1},
				},
			},
			want: "Validation failed:\n  Line 1: test error\n",
		},
		{
			name: "multiple errors",
			result: &ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Message: "error 1", Line: 1},
					{Message: "error 2"},
				},
			},
			want: "Validation failed:\n  Line 1: error 1\n  error 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatErrors(tt.result)
			if got != tt.want {
				t.Errorf("FormatErrors() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatWarnings(t *testing.T) {
	tests := []struct {
		name   string
		result *ValidationResult
		want   string
	}{
		{
			name: "no warnings",
			result: &ValidationResult{
				Warnings: []ValidationWarning{},
			},
			want: "",
		},
		{
			name: "single warning",
			result: &ValidationResult{
				Warnings: []ValidationWarning{
					{Message: "test warning"},
				},
			},
			want: "Warnings:\n  test warning\n",
		},
		{
			name: "warning with suggestion",
			result: &ValidationResult{
				Warnings: []ValidationWarning{
					{Message: "test warning", Suggestion: "do this instead"},
				},
			},
			want: "Warnings:\n  test warning\n    Suggestion: do this instead\n",
		},
		{
			name: "multiple warnings",
			result: &ValidationResult{
				Warnings: []ValidationWarning{
					{Message: "warning 1", Suggestion: "suggestion 1"},
					{Message: "warning 2"},
				},
			},
			want: "Warnings:\n  warning 1\n    Suggestion: suggestion 1\n  warning 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatWarnings(tt.result)
			if got != tt.want {
				t.Errorf("FormatWarnings() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidator_ValidateRule_UnknownType(t *testing.T) {
	v := &validator{}
	result := &ValidationResult{Valid: true}

	rule := ValidationRule{
		Type:    "unknown",
		Message: "test",
	}

	err := v.validateRule("test message", rule, result)
	if err != nil {
		t.Errorf("validateRule() with unknown type should not error: %v", err)
	}
	// Unknown rule types should be ignored
	if len(result.Errors) != 0 {
		t.Errorf("validateRule() with unknown type should not add errors")
	}
}

func TestValidator_ValidateLength_InvalidPattern(t *testing.T) {
	v := &validator{}
	result := &ValidationResult{Valid: true}

	rule := ValidationRule{
		Type:    "length",
		Pattern: "invalid",
	}

	err := v.validateLength("test message", rule, result)
	if err != nil {
		t.Errorf("validateLength() with invalid pattern should not error: %v", err)
	}
	// Invalid patterns should be ignored
	if len(result.Errors) != 0 {
		t.Errorf("validateLength() with invalid pattern should not add errors")
	}
}

func TestValidator_ValidatePattern_EmptyPattern(t *testing.T) {
	v := &validator{}
	result := &ValidationResult{Valid: true}

	rule := ValidationRule{
		Type:    "pattern",
		Pattern: "",
	}

	err := v.validatePattern("test message", rule, result)
	if err != nil {
		t.Errorf("validatePattern() with empty pattern should not error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Errorf("validatePattern() with empty pattern should not add errors")
	}
}

func TestValidator_ValidatePattern_InvalidRegex(t *testing.T) {
	v := &validator{}
	result := &ValidationResult{Valid: true}

	rule := ValidationRule{
		Type:    "pattern",
		Pattern: "[invalid(",
	}

	err := v.validatePattern("test message", rule, result)
	if err == nil {
		t.Error("validatePattern() with invalid regex should error")
	}
}

func TestValidator_ValidateRequired(t *testing.T) {
	v := &validator{}

	tests := []struct {
		name       string
		message    string
		wantErrors int
	}{
		{
			name:       "valid message",
			message:    "test message",
			wantErrors: 0,
		},
		{
			name:       "empty message",
			message:    "",
			wantErrors: 1,
		},
		{
			name:       "whitespace only",
			message:    "   \n  \t  ",
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{Valid: true}
			rule := ValidationRule{Type: "required"}

			err := v.validateRequired(tt.message, rule, result)
			if err != nil {
				t.Fatalf("validateRequired() error: %v", err)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("validateRequired() errors = %d, want %d", len(result.Errors), tt.wantErrors)
			}
		})
	}
}

func TestNewValidatorWithTemplateManager(t *testing.T) {
	mgr := NewTemplateManager()
	v := NewValidatorWithTemplateManager(mgr)

	if v == nil {
		t.Error("NewValidatorWithTemplateManager() returned nil")
	}
}
