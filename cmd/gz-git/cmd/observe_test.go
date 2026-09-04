package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/internal/testutil/builders"
	"github.com/gizzahub/gzh-cli-gitforge/pkg/contextref"
)

func TestObserveCommandJSONAndExit(t *testing.T) {
	dir := builders.NewGitRepoBuilder(t).WithFile("README.md", "hi\n").Build()
	cmd := newObserveCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{dir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var obs contextref.Observation
	if err := json.Unmarshal(stdout.Bytes(), &obs); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if obs.CapabilityID != contextref.CapabilityID {
		t.Fatalf("capability = %s", obs.CapabilityID)
	}
	if obs.ReleasedCETag != "v0.8.3" {
		t.Fatalf("tag = %s", obs.ReleasedCETag)
	}
	if obs.Context.Reason != contextref.ReasonNotDeclared {
		t.Fatalf("reason = %s", obs.Context.Reason)
	}
	if strings.Contains(stdout.String(), "native-discovery") {
		t.Fatal("native-discovery must not appear")
	}
}
