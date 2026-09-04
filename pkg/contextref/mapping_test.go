package contextref

import (
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

func TestMapCEExit(t *testing.T) {
	tests := []struct {
		name   string
		exit   int
		status string
		gz     int
		out    string
		domain string
	}{
		{"pass", 0, "adopted", cliutil.ExitOK, OutcomeObserved, ""},
		{"finding", 1, "not-adopted", cliutil.ExitOK, OutcomeObserved, ""},
		{"finding drift", 1, "drift", cliutil.ExitOK, OutcomeObserved, ""},
		{"ce fault", 2, "", cliutil.ExitToolError, OutcomeFault, DomainCEInvocation},
		{"disagree 0", 0, "not-adopted", cliutil.ExitToolError, OutcomeFault, DomainTransport},
		{"disagree 1", 1, "adopted", cliutil.ExitToolError, OutcomeFault, DomainTransport},
		{"other exit", 3, "adopted", cliutil.ExitToolError, OutcomeFault, DomainTransport},
		{"never partial", 2, "not-adopted", cliutil.ExitToolError, OutcomeFault, DomainCEInvocation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gz, out, domain := MapCEExit(tt.exit, tt.status)
			if gz != tt.gz || out != tt.out || domain != tt.domain {
				t.Fatalf("MapCEExit(%d, %q) = %d %s %s, want %d %s %s",
					tt.exit, tt.status, gz, out, domain, tt.gz, tt.out, tt.domain)
			}
			if gz == cliutil.ExitPartialFailed || gz == cliutil.ExitReclaimIncomplete {
				t.Fatal("D6 forbids gz-git exit 2 or 3")
			}
		})
	}
}
