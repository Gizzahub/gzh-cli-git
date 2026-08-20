package conformance

import (
	"path/filepath"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/integrate"
)

func TestResolverMatchesCommittedCorpus(t *testing.T) {
	rows, err := LoadFile(filepath.Join("testdata", "integration-branch-corpus.tsv"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if missing := MissingRequiredIDs(rows); len(missing) > 0 {
		t.Fatalf("corpus missing required row ids %v", missing)
	}
	for _, row := range rows {
		var remotes []string
		if row.Remote != "" {
			remotes = []string{row.Remote}
		}
		var cfg []string
		if row.Config != "" {
			cfg = []string{row.Config}
		}
		got := integrate.ResolveFromFacts(integrate.Facts{
			Config:      cfg,
			Refs:        row.Refs,
			Remotes:     remotes,
			DefaultName: row.DefaultName,
		})
		if got.Participates != row.Participates || got.Name != row.BareName {
			t.Errorf("row %s: got participates=%v name=%q, want participates=%v name=%q",
				row.ID, got.Participates, got.Name, row.Participates, row.BareName)
		}
	}
}
