// Package commit provides commit message automation and validation.
// This package includes template-based commit message generation, validation
// against conventional commit standards, and smart push operations with safety checks.
//
// Example usage:
//
//	// Generate commit message
//	gen := commit.NewGenerator()
//	msg, err := gen.Generate(ctx, repo, commit.GenerateOptions{
//	    Template: tmpl,
//	})
//
//	// Validate message
//	validator := commit.NewValidator()
//	result, err := validator.Validate(ctx, msg, tmpl)
package commit

import "errors"

// Template errors
var (
	ErrTemplateNotFound = errors.New("template not found")
	ErrInvalidTemplate  = errors.New("invalid template format")
	ErrValidationFailed = errors.New("message validation failed")
	ErrPushBlocked      = errors.New("push blocked by safety check")
	ErrNoChanges        = errors.New("no changes to commit")
)

// CommitError provides rich error context.
type CommitError struct {
	Op      string   // Operation (load, validate, push)
	Cause   error    // Underlying cause
	Message string   // Human-readable message
	Hints   []string // Suggestions to fix
}

// Error implements the error interface.
func (e *CommitError) Error() string {
	if e.Cause != nil {
		return e.Op + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Op + ": " + e.Message
}

// Unwrap implements error unwrapping.
func (e *CommitError) Unwrap() error {
	return e.Cause
}
