// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestReadYesNo covers the affirmative-answer parsing. Only "y"/"yes"
// (case-insensitive) proceed; everything else — including a bare newline or EOF
// — is treated as No, the safe default for destructive operations.
func TestReadYesNo(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"lowercase y", "y\n", true},
		{"uppercase Y", "Y\n", true},
		{"word yes", "yes\n", true},
		{"uppercase YES", "YES\n", true},
		{"padded yes", "  yes  \n", true},
		{"lowercase n", "n\n", false},
		{"word no", "no\n", false},
		{"bare newline defaults to no", "\n", false},
		{"empty EOF defaults to no", "", false},
		{"garbage defaults to no", "maybe\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readYesNo(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("readYesNo(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("readYesNo(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestConfirmDestructiveBulk_AssumeYes verifies that --yes short-circuits the
// gate and proceeds without touching stdin.
func TestConfirmDestructiveBulk_AssumeYes(t *testing.T) {
	proceed, err := confirmDestructiveBulk(true)
	if err != nil {
		t.Fatalf("confirmDestructiveBulk(true) error = %v", err)
	}
	if !proceed {
		t.Error("confirmDestructiveBulk(true) = false, want true")
	}
}

// TestConfirmDestructiveBulk_NonInteractiveRefuses verifies the core safety
// guarantee: without --yes, a non-interactive (piped/CI) stdin is refused rather
// than silently proceeding or blocking on a prompt. stdin is redirected to a
// pipe so the result is deterministic whether or not the test host has a TTY.
func TestConfirmDestructiveBulk_NonInteractiveRefuses(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer r.Close()
	defer w.Close()

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	proceed, err := confirmDestructiveBulk(false)
	if err == nil {
		t.Fatal("confirmDestructiveBulk(false) on non-interactive stdin: expected refusal error, got nil")
	}
	if proceed {
		t.Error("confirmDestructiveBulk(false) proceeded on non-interactive stdin, want refusal")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("refusal error = %q, want it to mention --yes", err.Error())
	}
}

// TestWithInterruptCancel_CancelsOnSignal verifies that a SIGTERM cancels the
// derived context, which is what lets clean/commit/cleanup stop gracefully and
// report partial results instead of being hard-killed.
func TestWithInterruptCancel_CancelsOnSignal(t *testing.T) {
	ctx, cancel := withInterruptCancel(context.Background())
	defer cancel()

	// signal.Notify is registered before withInterruptCancel returns, so the
	// handler is already installed by the time we raise the signal.
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("failed to raise SIGTERM: %v", err)
	}

	select {
	case <-ctx.Done():
		// success — the signal canceled the context
	case <-time.After(2 * time.Second):
		t.Fatal("context was not canceled within 2s of SIGTERM")
	}
}
