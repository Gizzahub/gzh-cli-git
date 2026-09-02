package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type failingCapabilityWriter struct{}

func (failingCapabilityWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestCapabilityCommandReportsSupportedCapability(t *testing.T) {
	for _, capability := range []string{
		integrateReadinessV1Capability,
		integrateQueueControllerV1Capability,
		integrateQueueBaseMissingV1Capability,
	} {
		t.Run(capability, func(t *testing.T) {
			cmd := newCapabilityCommand()
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetArgs([]string{capability})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got, want := stdout.String(), capability+"\n"; got != want {
				t.Fatalf("stdout = %q, want %q", got, want)
			}
		})
	}
}

func TestCapabilityCommandFailsWhenOutputCannotBeWritten(t *testing.T) {
	cmd := newCapabilityCommand()
	cmd.SetOut(failingCapabilityWriter{})
	cmd.SetArgs([]string{integrateQueueControllerV1Capability})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "write capability result") {
		t.Fatalf("Execute() error = %v, want output failure", err)
	}
}

func TestCapabilityCommandFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown", args: []string{"future-capability"}, want: "unsupported capability"},
		{name: "missing", args: nil, want: "accepts 1 arg"},
		{name: "extra", args: []string{integrateQueueControllerV1Capability, "extra"}, want: "accepts 1 arg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newCapabilityCommand()
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
