package conformance

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCommittedCorpusRequiresDivergenceIDs(t *testing.T) {
	rows, err := LoadFile(filepath.Join("testdata", "integration-branch-corpus.tsv"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if missing := MissingRequiredIDs(rows); len(missing) > 0 {
		t.Fatalf("corpus missing required row ids %v", missing)
	}
}

func TestMissingRequiredIDsReportsAbsentRows(t *testing.T) {
	rows := []Row{{ID: "A"}, {ID: "C"}}
	got := MissingRequiredIDs(rows)
	want := []string{"B", "D"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("MissingRequiredIDs = %v, want %v", got, want)
	}
}

func TestLoadParsesDivergenceRows(t *testing.T) {
	const table = "" +
		"id\tconfig\trefs\tremote\tdefault_name\tparticipates\tbare_name\n" +
		"A\t\trefs/remotes/upstream/develop\tupstream\tdevelop\ttrue\tdevelop\n" +
		"B\t\trefs/heads/develop\t\t\tfalse\t\n" +
		"C\tdevelopp\trefs/remotes/origin/develop\torigin\t\tfalse\t\n" +
		"D\trelease/2.0\trefs/remotes/origin/release/2.0\torigin\t\ttrue\trelease/2.0\n"

	rows, err := Load(strings.NewReader(table))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if missing := MissingRequiredIDs(rows); len(missing) > 0 {
		t.Fatalf("parsed table missing %v", missing)
	}

	byID := map[string]Row{}
	for _, row := range rows {
		byID[row.ID] = row
	}

	if got := byID["A"]; got.Remote != "upstream" || got.DefaultName != "develop" || got.BareName != "develop" || !got.Participates {
		t.Errorf("row A = %+v, want remote=upstream default=develop bare=develop participates=true", got)
	}
	if got := byID["B"]; got.Remote != "" || len(got.Refs) != 1 || got.Refs[0] != "refs/heads/develop" {
		t.Errorf("row B = %+v, want empty remote and local develop ref", got)
	}
	if got := byID["C"]; got.Config != "developp" || got.Participates {
		t.Errorf("row C = %+v, want typo config and participates=false", got)
	}
	if got := byID["D"]; got.Config != "release/2.0" || got.BareName != "release/2.0" {
		t.Errorf("row D = %+v, want bare_name release/2.0 (first slash kept)", got)
	}
}

func TestLoadRejectsBadHeader(t *testing.T) {
	_, err := Load(strings.NewReader("nope\ttoo\tshort\n"))
	if err == nil {
		t.Fatal("Load(bad header) = nil, want error")
	}
}
