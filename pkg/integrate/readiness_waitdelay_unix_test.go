//go:build darwin || linux

// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestExecuteReadinessWaitDelayBoundsEscapedPipe(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "escaped.pid")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("READINESS_TEST_BINARY", binary)
	t.Setenv("READINESS_ESCAPE_HELPER", "1")
	t.Setenv("READINESS_ESCAPE_PID_FILE", pidFile)
	runner := filepath.Join(dir, "runner")
	body := "#!/bin/sh\n\"$READINESS_TEST_BINARY\" -test.run=TestReadinessEscapedPipeHelper &\nwait\n"
	if err := os.WriteFile(runner, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	started := time.Now()
	go func() {
		_, err := executeReadinessWithTimeout(ctx, runner, dir, dir, "a", "b", 10*time.Second)
		done <- err
	}()
	pid := waitForReadinessPID(t, pidFile)
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel result = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > readinessWaitDelay+time.Second {
		t.Fatalf("escaped pipe exceeded bounded wait: %s", elapsed)
	}
}

func TestReadinessEscapedPipeHelper(t *testing.T) {
	if os.Getenv("READINESS_ESCAPE_HELPER") != "1" {
		return
	}
	if err := syscall.Setpgid(0, 0); err != nil {
		os.Exit(2)
	}
	pidFile := os.Getenv("READINESS_ESCAPE_PID_FILE")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		os.Exit(3)
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func waitForReadinessPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(path)
		if pid, err := strconv.Atoi(string(data)); err == nil && pid > 0 {
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("escaped helper did not publish pid")
	return 0
}
