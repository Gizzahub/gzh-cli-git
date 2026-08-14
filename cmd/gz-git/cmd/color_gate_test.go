package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"
)

// escByte is the ANSI escape introducer (0x1b). Colored output always contains
// at least one; a fully de-colored stream contains none.
const escByte = 0x1b

// TestHelpHasNoANSIWhenNotTerminal builds the real binary and runs `--help`
// with its stdout attached to a pipe (never a TTY under exec). The color gate
// must therefore emit zero escape bytes — covering AC1 (NO_COLOR=1) and AC2
// (non-TTY stdout). Guarded by -short because it compiles the binary.
func TestHelpHasNoANSIWhenNotTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: builds the gz-git binary")
	}

	bin := filepath.Join(t.TempDir(), "gz-git")
	if out, err := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "github.com/gizzahub/gzh-cli-gitforge/cmd/gz-git").CombinedOutput(); err != nil {
		t.Fatalf("build gz-git: %v\n%s", err, out)
	}

	cases := []struct {
		name string
		env  []string
	}{
		{"piped stdout is not a terminal", nil},
		{"NO_COLOR overrides everything", []string{"NO_COLOR=1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := exec.CommandContext(t.Context(), bin, "--help")
			c.Env = append(os.Environ(), tc.env...)
			out, err := c.CombinedOutput()
			if err != nil {
				t.Fatalf("gz-git --help: %v\n%s", err, out)
			}
			if i := bytes.IndexByte(out, escByte); i >= 0 {
				t.Errorf("found ANSI escape (0x1b) at byte %d; help output must be plain when not a terminal.\n%s", i, out)
			}
		})
	}
}

// TestUsageTemplateColorGate exercises both directions of the gate in-process:
// disabled → no escape bytes, enabled → escape bytes present. The enabled case
// is the proxy for AC3 (a real TTY keeps its colors) that a piped test cannot
// observe directly.
func TestUsageTemplateColorGate(t *testing.T) {
	t.Cleanup(func() {
		if cliutil.ColorsEnabled() {
			cliutil.EnableColors()
		} else {
			cliutil.DisableColors()
		}
	})

	cliutil.DisableColors()
	if bytes.ContainsRune([]byte(buildUsageTemplate()), escByte) {
		t.Error("usage template contains ANSI escape while colors are disabled")
	}

	cliutil.EnableColors()
	if !bytes.ContainsRune([]byte(buildUsageTemplate()), escByte) {
		t.Error("usage template missing ANSI escape while colors are enabled")
	}
}
