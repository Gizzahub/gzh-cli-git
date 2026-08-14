// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package cliutil

import (
	"errors"
	"fmt"
)

// Exit code contract shared by gz-git commands.
//
// One-shot bulk commands (clone, update, pull, fetch, push, status, commit,
// switch, stash, tag, clean, diff):
//
//	0  all repositories succeeded
//	1  tool or configuration error (bad flag, scan failure)
//	2  completed, but one or more repositories failed
//
// Reclaim after a successful integrate:
//
//	3  integrate succeeded, reclaim did not finish
//
// Diagnostic commands follow the grep convention instead (e.g.
// `conflict detect`): 0 = nothing found, 1 = findings, 2 = execution error.
const (
	ExitOK                = 0
	ExitToolError         = 1
	ExitPartialFailed     = 2
	ExitReclaimIncomplete = 3
)

// ExitError carries a process exit code alongside an error so a command's RunE
// can signal outcomes beyond the default failure code (1). root.Execute inspects
// it with errors.As and exits with Code; any error that is not an *ExitError
// keeps the default exit code 1.
type ExitError struct {
	Code int
	Err  error
}

// NewExitError wraps err with an explicit process exit code. It returns nil when
// err is nil so call sites can `return cliutil.NewExitError(code, mkErr())`
// without a preceding nil check.
func NewExitError(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: code, Err: err}
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying error for errors.Is/As chains.
func (e *ExitError) Unwrap() error { return e.Err }

// ExitCodeForError maps a command error to a process exit code: nil → 0, an
// *ExitError (anywhere in the chain) → its Code, any other error → 1.
func ExitCodeForError(err error) int {
	if err == nil {
		return ExitOK
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return ExitToolError
}
