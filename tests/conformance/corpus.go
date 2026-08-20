// Package conformance holds the committed integration-branch corpus and its
// parser. It does not interpret the rows.
package conformance

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// RequiredIDs are the DECISION-004 divergence rows. A parser test must fail
// if any of these are missing from the committed table.
var RequiredIDs = []string{"A", "B", "C", "D"}

// Row is one corpus case: (config, refs, remote, default name) → (participates, bare name).
type Row struct {
	ID           string
	Config       string
	Refs         []string
	Remote       string
	DefaultName  string
	Participates bool
	BareName     string
}

// LoadFile reads a TSV corpus from path.
func LoadFile(path string) ([]Row, error) {
	f, err := os.Open(path) // #nosec G304 -- test corpus path is explicitly supplied by the test caller.
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Load(f)
}

// Load reads a TSV corpus. Comment lines starting with # are skipped.
func Load(r io.Reader) ([]Row, error) {
	cr := csv.NewReader(r)
	cr.Comma = '\t'
	cr.Comment = '#'
	cr.FieldsPerRecord = 7
	cr.ReuseRecord = false
	cr.LazyQuotes = true

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read corpus header: %w", err)
	}
	wantHeader := []string{"id", "config", "refs", "remote", "default_name", "participates", "bare_name"}
	if !equalStrings(header, wantHeader) {
		return nil, fmt.Errorf("corpus header %q, want %q", header, wantHeader)
	}

	var rows []Row
	line := 1 // header consumed
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read corpus row: %w", err)
		}
		line++
		row, err := parseRow(rec)
		if err != nil {
			return nil, fmt.Errorf("corpus line %d: %w", line, err)
		}
		rows = append(rows, row)
	}
}

// MissingRequiredIDs returns RequiredIDs not present in rows.
func MissingRequiredIDs(rows []Row) []string {
	have := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		have[row.ID] = struct{}{}
	}
	var missing []string
	for _, id := range RequiredIDs {
		if _, ok := have[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing
}

func parseRow(rec []string) (Row, error) {
	participates, err := strconv.ParseBool(rec[5])
	if err != nil {
		return Row{}, fmt.Errorf("id %q: participates %q: %w", rec[0], rec[5], err)
	}
	if rec[0] == "" {
		return Row{}, fmt.Errorf("empty id")
	}
	return Row{
		ID:           rec[0],
		Config:       rec[1],
		Refs:         splitRefs(rec[2]),
		Remote:       rec[3],
		DefaultName:  rec[4],
		Participates: participates,
		BareName:     rec[6],
	}, nil
}

func splitRefs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
