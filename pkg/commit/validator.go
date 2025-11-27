package commit

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Validator validates commit messages.
type Validator interface {
	// Validate checks if message follows template rules.
	Validate(ctx context.Context, message string, template *Template) (*ValidationResult, error)

	// ValidateInteractive validates with user interaction.
	ValidateInteractive(ctx context.Context, message string) (*ValidationResult, error)
}

// ValidationResult contains validation results.
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationWarning
}

// ValidationError represents a validation error.
type ValidationError struct {
	Rule    string
	Message string
	Line    int
	Column  int
}

// ValidationWarning represents a validation warning.
type ValidationWarning struct {
	Message    string
	Suggestion string
}

// validator implements Validator.
type validator struct {
	templateMgr TemplateManager
}

// NewValidator creates a new Validator.
func NewValidator() Validator {
	return &validator{
		templateMgr: NewTemplateManager(),
	}
}

// NewValidatorWithTemplateManager creates a new Validator with a custom TemplateManager.
func NewValidatorWithTemplateManager(mgr TemplateManager) Validator {
	return &validator{
		templateMgr: mgr,
	}
}

// Validate checks if message follows template rules.
func (v *validator) Validate(ctx context.Context, message string, tmpl *Template) (*ValidationResult, error) {
	if message == "" {
		return &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{Message: "commit message cannot be empty"},
			},
		}, nil
	}

	if tmpl == nil {
		return &ValidationResult{
			Valid: false,
			Errors: []ValidationError{
				{Message: "template cannot be nil"},
			},
		}, nil
	}

	result := &ValidationResult{
		Valid:    true,
		Errors:   []ValidationError{},
		Warnings: []ValidationWarning{},
	}

	// Validate against each rule
	for _, rule := range tmpl.Rules {
		if err := v.validateRule(message, rule, result); err != nil {
			return nil, fmt.Errorf("failed to validate rule: %w", err)
		}
	}

	// Add warnings for common issues
	v.addWarnings(message, result)

	// Set overall validity
	result.Valid = len(result.Errors) == 0

	return result, nil
}

// ValidateInteractive validates with user interaction.
func (v *validator) ValidateInteractive(ctx context.Context, message string) (*ValidationResult, error) {
	// Load conventional commits template by default
	tmpl, err := v.templateMgr.Load(ctx, "conventional")
	if err != nil {
		return nil, fmt.Errorf("failed to load default template: %w", err)
	}

	return v.Validate(ctx, message, tmpl)
}

// validateRule validates a single rule against the message.
func (v *validator) validateRule(message string, rule ValidationRule, result *ValidationResult) error {
	switch rule.Type {
	case "length":
		return v.validateLength(message, rule, result)
	case "pattern":
		return v.validatePattern(message, rule, result)
	case "required":
		return v.validateRequired(message, rule, result)
	default:
		// Unknown rule types are ignored
		return nil
	}
}

// validateLength validates message length.
func (v *validator) validateLength(message string, rule ValidationRule, result *ValidationResult) error {
	// Extract first line (subject)
	lines := strings.Split(message, "\n")
	subject := strings.TrimSpace(lines[0])

	// Parse pattern to extract length constraint
	// Pattern format: "^.{min,max}$" or "^.{1,72}$"
	re := regexp.MustCompile(`\{(\d+),(\d+)\}`)
	matches := re.FindStringSubmatch(rule.Pattern)

	if len(matches) != 3 {
		// If pattern doesn't match expected format, skip validation
		return nil
	}

	var min, max int
	fmt.Sscanf(matches[1], "%d", &min)
	fmt.Sscanf(matches[2], "%d", &max)

	subjectLen := utf8.RuneCountInString(subject)

	if subjectLen < min || subjectLen > max {
		errMsg := rule.Message
		if errMsg == "" {
			errMsg = fmt.Sprintf("subject line must be %d-%d characters (currently %d)", min, max, subjectLen)
		}
		result.Errors = append(result.Errors, ValidationError{
			Rule:    "length",
			Message: errMsg,
			Line:    1,
		})
	}

	return nil
}

// validatePattern validates message against regex pattern.
func (v *validator) validatePattern(message string, rule ValidationRule, result *ValidationResult) error {
	if rule.Pattern == "" {
		return nil
	}

	// Compile regex
	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	// Extract first line for pattern matching
	lines := strings.Split(message, "\n")
	subject := strings.TrimSpace(lines[0])

	// Check if pattern matches
	if !re.MatchString(subject) {
		errMsg := rule.Message
		if errMsg == "" {
			errMsg = fmt.Sprintf("subject does not match required pattern: %s", rule.Pattern)
		}
		result.Errors = append(result.Errors, ValidationError{
			Rule:    "pattern",
			Message: errMsg,
			Line:    1,
		})
	}

	return nil
}

// validateRequired validates required fields.
func (v *validator) validateRequired(message string, rule ValidationRule, result *ValidationResult) error {
	if strings.TrimSpace(message) == "" {
		errMsg := rule.Message
		if errMsg == "" {
			errMsg = "commit message is required"
		}
		result.Errors = append(result.Errors, ValidationError{
			Rule:    "required",
			Message: errMsg,
		})
	}
	return nil
}

// addWarnings adds warnings for common issues.
func (v *validator) addWarnings(message string, result *ValidationResult) {
	lines := strings.Split(message, "\n")
	subject := strings.TrimSpace(lines[0])

	// Check for imperative mood
	if !v.isImperativeMood(subject) {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Message:    "subject may not be in imperative mood",
			Suggestion: "use imperative mood (e.g., 'add' not 'added', 'fix' not 'fixed')",
		})
	}

	// Check for period at end of subject
	if strings.HasSuffix(subject, ".") {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Message:    "subject ends with a period",
			Suggestion: "remove the period at the end of the subject line",
		})
	}

	// Check for capitalization
	if len(subject) > 0 {
		firstChar := string([]rune(subject)[0])
		// Skip type prefixes like "feat:", "fix:"
		if strings.Contains(subject, ":") {
			parts := strings.SplitN(subject, ":", 2)
			if len(parts) == 2 {
				desc := strings.TrimSpace(parts[1])
				if len(desc) > 0 {
					firstChar = string([]rune(desc)[0])
					if firstChar == strings.ToUpper(firstChar) && firstChar != strings.ToLower(firstChar) {
						result.Warnings = append(result.Warnings, ValidationWarning{
							Message:    "description starts with uppercase letter",
							Suggestion: "use lowercase for description after type/scope",
						})
					}
				}
			}
		}
	}

	// Check for very long body lines
	for i, line := range lines {
		if i == 0 {
			continue // Skip subject
		}
		if utf8.RuneCountInString(line) > 100 && strings.TrimSpace(line) != "" {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Message:    fmt.Sprintf("line %d is very long (%d characters)", i+1, utf8.RuneCountInString(line)),
				Suggestion: "consider wrapping body lines at 72-100 characters",
			})
			break // Only warn once
		}
	}
}

// isImperativeMood checks if the subject uses imperative mood.
func (v *validator) isImperativeMood(subject string) bool {
	// Common past tense suffixes
	pastTenseSuffixes := []string{"ed", "ing"}

	// Extract description after type/scope
	desc := subject
	if strings.Contains(subject, ":") {
		parts := strings.SplitN(subject, ":", 2)
		if len(parts) == 2 {
			desc = strings.TrimSpace(parts[1])
		}
	}

	// Get first word
	words := strings.Fields(desc)
	if len(words) == 0 {
		return true // Can't determine, assume valid
	}

	firstWord := strings.ToLower(words[0])

	// Check for common past tense patterns
	for _, suffix := range pastTenseSuffixes {
		if strings.HasSuffix(firstWord, suffix) {
			// Exclude words that are naturally in -ed/-ing form
			exceptions := map[string]bool{
				"add":    true,
				"need":   true,
				"feed":   true,
				"during": true,
				"string": true,
			}
			if !exceptions[firstWord] {
				return false
			}
		}
	}

	return true
}

// FormatErrors formats validation errors as a human-readable string.
func FormatErrors(result *ValidationResult) string {
	if result.Valid {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Validation failed:\n")

	for _, err := range result.Errors {
		if err.Line > 0 {
			sb.WriteString(fmt.Sprintf("  Line %d: %s\n", err.Line, err.Message))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", err.Message))
		}
	}

	return sb.String()
}

// FormatWarnings formats validation warnings as a human-readable string.
func FormatWarnings(result *ValidationResult) string {
	if len(result.Warnings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Warnings:\n")

	for _, warn := range result.Warnings {
		sb.WriteString(fmt.Sprintf("  %s\n", warn.Message))
		if warn.Suggestion != "" {
			sb.WriteString(fmt.Sprintf("    Suggestion: %s\n", warn.Suggestion))
		}
	}

	return sb.String()
}
