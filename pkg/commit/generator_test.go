package commit

import (
	"context"
	"strings"
	"testing"
)

func TestGenerator_New(t *testing.T) {
	g := NewGenerator()
	if g == nil {
		t.Fatal("NewGenerator() returned nil")
	}
}

func TestGenerator_Suggest(t *testing.T) {
	ctx := context.Background()
	g := NewGenerator()

	tests := []struct {
		name        string
		changes     *DiffSummary
		wantType    string
		wantMinConf float64
		wantErr     bool
	}{
		{
			name: "test files",
			changes: &DiffSummary{
				FilesChanged:  2,
				AddedFiles:    []string{"test/foo_test.go", "test/bar_test.go"},
				ModifiedFiles: []string{},
				DeletedFiles:  []string{},
			},
			wantType:    "test",
			wantMinConf: 0.7,
			wantErr:     false,
		},
		{
			name: "documentation files",
			changes: &DiffSummary{
				FilesChanged:  1,
				ModifiedFiles: []string{"README.md"},
			},
			wantType:    "docs",
			wantMinConf: 0.8,
			wantErr:     false,
		},
		{
			name: "new feature (more additions)",
			changes: &DiffSummary{
				FilesChanged:  3,
				AddedFiles:    []string{"feature1.go", "feature2.go"},
				ModifiedFiles: []string{"main.go"},
			},
			wantType:    "feat",
			wantMinConf: 0.6,
			wantErr:     false,
		},
		{
			name: "bug fix (more modifications)",
			changes: &DiffSummary{
				FilesChanged:  3,
				ModifiedFiles: []string{"bug1.go", "bug2.go", "bug3.go"},
				AddedFiles:    []string{},
			},
			wantType:    "fix",
			wantMinConf: 0.6,
			wantErr:     false,
		},
		{
			name: "configuration files",
			changes: &DiffSummary{
				FilesChanged:  2,
				ModifiedFiles: []string{"config.yaml", "settings.json"},
			},
			wantType:    "chore",
			wantMinConf: 0.5,
			wantErr:     false,
		},
		{
			name:    "nil changes",
			changes: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestion, err := g.Suggest(ctx, tt.changes)

			if tt.wantErr {
				if err == nil {
					t.Error("Suggest() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Suggest() unexpected error: %v", err)
			}

			if suggestion.Type != tt.wantType {
				t.Errorf("Suggest() type = %q, want %q", suggestion.Type, tt.wantType)
			}

			if suggestion.Confidence < tt.wantMinConf {
				t.Errorf("Suggest() confidence = %f, want >= %f", suggestion.Confidence, tt.wantMinConf)
			}

			if suggestion.Description == "" {
				t.Error("Suggest() description is empty")
			}
		})
	}
}

func TestGenerator_InferType(t *testing.T) {
	g := &generator{}

	tests := []struct {
		name        string
		changes     *DiffSummary
		wantType    string
		wantMinConf float64
	}{
		{
			name: "predominantly test files",
			changes: &DiffSummary{
				AddedFiles: []string{"foo_test.go", "bar_test.go"},
				ModifiedFiles: []string{"main.go"},
			},
			wantType:    "test",
			wantMinConf: 0.7,
		},
		{
			name: "predominantly docs",
			changes: &DiffSummary{
				ModifiedFiles: []string{"README.md", "CONTRIBUTING.md"},
				AddedFiles: []string{"main.go"},
			},
			wantType:    "docs",
			wantMinConf: 0.8,
		},
		{
			name: "more additions than deletions",
			changes: &DiffSummary{
				AddedFiles:    []string{"new1.go", "new2.go", "new3.go"},
				DeletedFiles:  []string{"old.go"},
				ModifiedFiles: []string{},
			},
			wantType:    "feat",
			wantMinConf: 0.6,
		},
		{
			name: "more modifications",
			changes: &DiffSummary{
				ModifiedFiles: []string{"fix1.go", "fix2.go", "fix3.go"},
				AddedFiles:    []string{},
				DeletedFiles:  []string{},
			},
			wantType:    "fix",
			wantMinConf: 0.6,
		},
		{
			name: "config files",
			changes: &DiffSummary{
				ModifiedFiles: []string{"config.yaml", "settings.json"},
			},
			wantType:    "chore",
			wantMinConf: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotConf := g.inferType(tt.changes)

			if gotType != tt.wantType {
				t.Errorf("inferType() type = %q, want %q", gotType, tt.wantType)
			}

			if gotConf < tt.wantMinConf {
				t.Errorf("inferType() confidence = %f, want >= %f", gotConf, tt.wantMinConf)
			}
		})
	}
}

func TestGenerator_InferScope(t *testing.T) {
	g := &generator{}

	tests := []struct {
		name    string
		changes *DiffSummary
		want    string
	}{
		{
			name: "pkg directory - second level wins alphabetically",
			changes: &DiffSummary{
				FilesChanged:  2,
				ModifiedFiles: []string{"pkg/commit/template.go", "pkg/commit/validator.go"},
			},
			want: "commit", // "commit" < "pkg" alphabetically, both have count 2
		},
		{
			name: "cmd directory - alphabetically first wins when equal count",
			changes: &DiffSummary{
				FilesChanged:  2,
				ModifiedFiles: []string{"cmd/cli/main.go", "cmd/cli/flags.go"},
			},
			want: "cli", // "cli" < "cmd" alphabetically, both have count 2
		},
		{
			name: "simple directory",
			changes: &DiffSummary{
				FilesChanged:  1,
				ModifiedFiles: []string{"docs/guide.md"},
			},
			want: "docs",
		},
		{
			name: "root level files",
			changes: &DiffSummary{
				FilesChanged:  2,
				ModifiedFiles: []string{"README.md", "go.mod"},
			},
			want: "",
		},
		{
			name: "no files",
			changes: &DiffSummary{
				FilesChanged: 0,
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.inferScope(tt.changes)

			if got != tt.want {
				t.Errorf("inferScope() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerator_GenerateDescription(t *testing.T) {
	g := &generator{}

	tests := []struct {
		name       string
		changes    *DiffSummary
		commitType string
		wantContains string
	}{
		{
			name: "feat with single file",
			changes: &DiffSummary{
				FilesChanged: 1,
				AddedFiles:   []string{"feature.go"},
			},
			commitType:   "feat",
			wantContains: "add feature",
		},
		{
			name: "feat with multiple files",
			changes: &DiffSummary{
				FilesChanged: 3,
				AddedFiles:   []string{"f1.go", "f2.go", "f3.go"},
			},
			commitType:   "feat",
			wantContains: "add new features",
		},
		{
			name: "fix with single file",
			changes: &DiffSummary{
				FilesChanged:  1,
				ModifiedFiles: []string{"buggy.go"},
			},
			commitType:   "fix",
			wantContains: "fix buggy",
		},
		{
			name: "docs with single file",
			changes: &DiffSummary{
				FilesChanged:  1,
				ModifiedFiles: []string{"README.md"},
			},
			commitType:   "docs",
			wantContains: "update readme",
		},
		{
			name: "test files",
			changes: &DiffSummary{
				FilesChanged: 2,
				AddedFiles:   []string{"test1.go", "test2.go"},
			},
			commitType:   "test",
			wantContains: "add tests",
		},
		{
			name: "refactor single file",
			changes: &DiffSummary{
				FilesChanged:  1,
				ModifiedFiles: []string{"code.go"},
			},
			commitType:   "refactor",
			wantContains: "refactor code",
		},
		{
			name: "chore",
			changes: &DiffSummary{
				FilesChanged:  2,
				ModifiedFiles: []string{"config.yaml"},
			},
			commitType:   "chore",
			wantContains: "update configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.generateDescription(tt.changes, tt.commitType)

			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("generateDescription() = %q, want to contain %q", got, tt.wantContains)
			}
		})
	}
}

func TestGenerator_ParseStats(t *testing.T) {
	g := &generator{}

	tests := []struct {
		name            string
		output          string
		wantInsertions  int
		wantDeletions   int
	}{
		{
			name:           "insertions and deletions",
			output:         "1 file changed, 10 insertions(+), 5 deletions(-)",
			wantInsertions: 10,
			wantDeletions:  5,
		},
		{
			name:           "only insertions",
			output:         "2 files changed, 25 insertions(+)",
			wantInsertions: 25,
			wantDeletions:  0,
		},
		{
			name:           "only deletions",
			output:         "1 file changed, 15 deletions(-)",
			wantInsertions: 0,
			wantDeletions:  15,
		},
		{
			name:           "no changes",
			output:         "",
			wantInsertions: 0,
			wantDeletions:  0,
		},
		{
			name:           "multiline output",
			output:         "file1.go | 10 +++++++\nfile2.go | 5 -----\n2 files changed, 10 insertions(+), 5 deletions(-)",
			wantInsertions: 10,
			wantDeletions:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIns, gotDel := g.parseStats(tt.output)

			if gotIns != tt.wantInsertions {
				t.Errorf("parseStats() insertions = %d, want %d", gotIns, tt.wantInsertions)
			}

			if gotDel != tt.wantDeletions {
				t.Errorf("parseStats() deletions = %d, want %d", gotDel, tt.wantDeletions)
			}
		})
	}
}

func TestDiffSummary_Struct(t *testing.T) {
	summary := &DiffSummary{
		FilesChanged:  3,
		Insertions:    10,
		Deletions:     5,
		ModifiedFiles: []string{"file1.go"},
		AddedFiles:    []string{"file2.go"},
		DeletedFiles:  []string{"file3.go"},
	}

	if summary.FilesChanged != 3 {
		t.Errorf("FilesChanged = %d, want 3", summary.FilesChanged)
	}

	if summary.Insertions != 10 {
		t.Errorf("Insertions = %d, want 10", summary.Insertions)
	}

	if summary.Deletions != 5 {
		t.Errorf("Deletions = %d, want 5", summary.Deletions)
	}

	if len(summary.ModifiedFiles) != 1 {
		t.Errorf("ModifiedFiles length = %d, want 1", len(summary.ModifiedFiles))
	}

	if len(summary.AddedFiles) != 1 {
		t.Errorf("AddedFiles length = %d, want 1", len(summary.AddedFiles))
	}

	if len(summary.DeletedFiles) != 1 {
		t.Errorf("DeletedFiles length = %d, want 1", len(summary.DeletedFiles))
	}
}

func TestSuggestion_Struct(t *testing.T) {
	suggestion := &Suggestion{
		Type:        "feat",
		Scope:       "api",
		Description: "add new endpoint",
		Confidence:  0.9,
	}

	if suggestion.Type != "feat" {
		t.Errorf("Type = %q, want %q", suggestion.Type, "feat")
	}

	if suggestion.Scope != "api" {
		t.Errorf("Scope = %q, want %q", suggestion.Scope, "api")
	}

	if suggestion.Description != "add new endpoint" {
		t.Errorf("Description = %q, want %q", suggestion.Description, "add new endpoint")
	}

	if suggestion.Confidence != 0.9 {
		t.Errorf("Confidence = %f, want 0.9", suggestion.Confidence)
	}
}

func TestGenerateOptions_Defaults(t *testing.T) {
	opts := GenerateOptions{}

	if opts.Template != nil {
		t.Error("default Template should be nil")
	}

	if opts.Interactive {
		t.Error("default Interactive should be false")
	}

	if opts.MaxLength != 0 {
		t.Errorf("default MaxLength = %d, want 0", opts.MaxLength)
	}
}
