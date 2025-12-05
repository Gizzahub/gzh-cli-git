package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestSimpleLogger(t *testing.T) {
	var buf bytes.Buffer
	log := New("test")
	log.SetOutput(&buf)
	log.SetLevel(LevelDebug)

	log.Debug("debug message")
	log.Info("info message")
	log.Warn("warn message")
	log.Error("error message")

	output := buf.String()

	if !strings.Contains(output, "DEBUG") {
		t.Error("expected DEBUG in output")
	}
	if !strings.Contains(output, "INFO") {
		t.Error("expected INFO in output")
	}
	if !strings.Contains(output, "WARN") {
		t.Error("expected WARN in output")
	}
	if !strings.Contains(output, "ERROR") {
		t.Error("expected ERROR in output")
	}
	if !strings.Contains(output, "[test]") {
		t.Error("expected logger name in output")
	}
}

func TestLogLevel(t *testing.T) {
	var buf bytes.Buffer
	log := New("test")
	log.SetOutput(&buf)
	log.SetLevel(LevelWarn)

	log.Debug("debug")
	log.Info("info")
	log.Warn("warn")

	output := buf.String()

	if strings.Contains(output, "DEBUG") {
		t.Error("DEBUG should be filtered out")
	}
	if strings.Contains(output, "INFO") {
		t.Error("INFO should be filtered out")
	}
	if !strings.Contains(output, "WARN") {
		t.Error("WARN should be present")
	}
}

func TestWithContext(t *testing.T) {
	var buf bytes.Buffer
	log := New("test")
	log.SetOutput(&buf)

	contextLog := log.WithContext("repo", "test-repo")
	contextLog.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "repo=test-repo") {
		t.Error("expected context in output")
	}
}

func TestLoggerWithArgs(t *testing.T) {
	var buf bytes.Buffer
	log := New("test")
	log.SetOutput(&buf)

	log.Info("git operation", "command", "clone", "status", "success")

	output := buf.String()
	if !strings.Contains(output, "command=clone") {
		t.Error("expected args in output")
	}
	if !strings.Contains(output, "status=success") {
		t.Error("expected args in output")
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("Level.String() = %v, want %v", got, tt.want)
		}
	}
}

func TestDefaultLogger(t *testing.T) {
	// Verify default logger exists and works.
	if Default == nil {
		t.Error("Default logger should not be nil")
	}

	// These should not panic.
	var buf bytes.Buffer
	Default.SetOutput(&buf)
	Default.SetLevel(LevelDebug)

	Debug("debug msg")
	Info("info msg")
	Warn("warn msg")
	Error("error msg")

	output := buf.String()
	if !strings.Contains(output, "[git]") {
		t.Error("expected default logger name 'git' in output")
	}
}
