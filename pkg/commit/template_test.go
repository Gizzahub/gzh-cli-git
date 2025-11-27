package commit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateManager_Load(t *testing.T) {
	ctx := context.Background()
	mgr := NewTemplateManager()

	tests := []struct {
		name    string
		tmplName string
		wantErr error
	}{
		{
			name:     "load conventional template",
			tmplName: "conventional",
			wantErr:  nil,
		},
		{
			name:     "load semantic template",
			tmplName: "semantic",
			wantErr:  nil,
		},
		{
			name:     "empty template name",
			tmplName: "",
			wantErr:  ErrTemplateNotFound,
		},
		{
			name:     "non-existent template",
			tmplName: "nonexistent",
			wantErr:  ErrTemplateNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := mgr.Load(ctx, tt.tmplName)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Load() expected error %v, got nil", tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if tmpl == nil {
				t.Fatal("Load() returned nil template")
			}
			if tmpl.Name != tt.tmplName {
				t.Errorf("Load() name = %s, want %s", tmpl.Name, tt.tmplName)
			}
			if tmpl.Format == "" {
				t.Error("Load() template has empty format")
			}
		})
	}
}

func TestTemplateManager_LoadCustom(t *testing.T) {
	ctx := context.Background()
	mgr := NewTemplateManager()

	// Create temporary template file
	tmpDir := t.TempDir()
	tmplPath := filepath.Join(tmpDir, "custom.yaml")

	customTemplate := `name: custom
description: Custom template
format: |
  {{.Type}}: {{.Message}}
variables:
  - name: Type
    type: string
    required: true
  - name: Message
    type: string
    required: true
rules:
  - type: pattern
    pattern: "^.+: .+$"
    message: "Must have format: Type: Message"
examples:
  - "CUSTOM: My custom message"
`

	if err := os.WriteFile(tmplPath, []byte(customTemplate), 0644); err != nil {
		t.Fatalf("Failed to create test template file: %v", err)
	}

	// Create invalid YAML file
	invalidYamlPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(invalidYamlPath, []byte("invalid: [yaml: content"), 0644); err != nil {
		t.Fatalf("Failed to create invalid YAML file: %v", err)
	}

	// Create relative path template
	relPath := "custom.yaml"
	relFullPath := filepath.Join(tmpDir, relPath)
	if err := os.WriteFile(relFullPath, []byte(customTemplate), 0644); err != nil {
		t.Fatalf("Failed to create relative path template: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantErr  error
		wantName string
		setup    func() (string, func())
	}{
		{
			name:     "load custom template",
			path:     tmplPath,
			wantErr:  nil,
			wantName: "custom",
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: ErrTemplateNotFound,
		},
		{
			name:    "non-existent file",
			path:    filepath.Join(tmpDir, "nonexistent.yaml"),
			wantErr: ErrTemplateNotFound,
		},
		{
			name:    "invalid YAML",
			path:    invalidYamlPath,
			wantErr: ErrInvalidTemplate,
		},
		{
			name:     "relative path",
			wantErr:  nil,
			wantName: "custom",
			setup: func() (string, func()) {
				// Save current dir
				origDir, _ := os.Getwd()
				// Change to temp dir
				os.Chdir(tmpDir)
				return relPath, func() { os.Chdir(origDir) }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path
			if tt.setup != nil {
				var cleanup func()
				path, cleanup = tt.setup()
				defer cleanup()
			}

			tmpl, err := mgr.LoadCustom(ctx, path)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("LoadCustom() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadCustom() unexpected error: %v", err)
			}
			if tmpl == nil {
				t.Fatal("LoadCustom() returned nil template")
			}
			if tmpl.Name != tt.wantName {
				t.Errorf("LoadCustom() name = %s, want %s", tmpl.Name, tt.wantName)
			}
		})
	}
}

func TestTemplateManager_List(t *testing.T) {
	ctx := context.Background()
	mgr := NewTemplateManager()

	names, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(names) == 0 {
		t.Error("List() returned empty list, expected built-in templates")
	}

	// Check for expected templates
	expectedTemplates := map[string]bool{
		"conventional": false,
		"semantic":     false,
	}

	for _, name := range names {
		if _, ok := expectedTemplates[name]; ok {
			expectedTemplates[name] = true
		}
	}

	for name, found := range expectedTemplates {
		if !found {
			t.Errorf("List() missing expected template: %s", name)
		}
	}
}

func TestTemplateManager_Validate(t *testing.T) {
	ctx := context.Background()
	mgr := NewTemplateManager()

	tests := []struct {
		name    string
		tmpl    *Template
		wantErr error
	}{
		{
			name: "valid template",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type}}: {{.Message}}",
				Variables: []TemplateVariable{
					{Name: "Type", Type: "string", Required: true},
					{Name: "Message", Type: "string", Required: true},
				},
			},
			wantErr: nil,
		},
		{
			name:    "nil template",
			tmpl:    nil,
			wantErr: ErrInvalidTemplate,
		},
		{
			name: "empty name",
			tmpl: &Template{
				Format: "{{.Message}}",
			},
			wantErr: ErrInvalidTemplate,
		},
		{
			name: "empty format",
			tmpl: &Template{
				Name: "test",
			},
			wantErr: ErrInvalidTemplate,
		},
		{
			name: "invalid template format",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type",
			},
			wantErr: ErrInvalidTemplate,
		},
		{
			name: "variable with empty name",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Message}}",
				Variables: []TemplateVariable{
					{Name: "", Type: "string"},
				},
			},
			wantErr: ErrInvalidTemplate,
		},
		{
			name: "variable with invalid type",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Message}}",
				Variables: []TemplateVariable{
					{Name: "Message", Type: "invalid"},
				},
			},
			wantErr: ErrInvalidTemplate,
		},
		{
			name: "enum without options",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type}}",
				Variables: []TemplateVariable{
					{Name: "Type", Type: "enum"},
				},
			},
			wantErr: ErrInvalidTemplate,
		},
		{
			name: "valid enum",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type}}",
				Variables: []TemplateVariable{
					{Name: "Type", Type: "enum", Options: []string{"feat", "fix"}},
				},
			},
			wantErr: nil,
		},
		{
			name: "pattern rule with invalid regex",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Message}}",
				Rules: []ValidationRule{
					{Type: "pattern", Pattern: "[invalid("},
				},
			},
			wantErr: ErrInvalidTemplate,
		},
		{
			name: "pattern rule without pattern",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Message}}",
				Rules: []ValidationRule{
					{Type: "pattern"},
				},
			},
			wantErr: ErrInvalidTemplate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.Validate(ctx, tt.tmpl)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Validate() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestTemplateManager_Render_NilTemplate(t *testing.T) {
	ctx := context.Background()
	mgr := NewTemplateManager()

	_, err := mgr.Render(ctx, nil, nil)
	if err == nil {
		t.Fatal("Render() with nil template expected error, got nil")
	}
}

func TestTemplateManager_Render_NilValues(t *testing.T) {
	ctx := context.Background()
	mgr := NewTemplateManager()

	tmpl := &Template{
		Name:   "test",
		Format: "static message",
	}

	got, err := mgr.Render(ctx, tmpl, nil)
	if err != nil {
		t.Fatalf("Render() with nil values unexpected error: %v", err)
	}
	if got != "static message" {
		t.Errorf("Render() = %q, want %q", got, "static message")
	}
}

func TestTemplateManager_Render(t *testing.T) {
	ctx := context.Background()
	mgr := NewTemplateManager()

	tests := []struct {
		name    string
		tmpl    *Template
		values  map[string]string
		want    string
		wantErr bool
	}{
		{
			name: "simple render",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type}}: {{.Message}}",
				Variables: []TemplateVariable{
					{Name: "Type", Type: "string", Required: true},
					{Name: "Message", Type: "string", Required: true},
				},
			},
			values: map[string]string{
				"Type":    "feat",
				"Message": "add feature",
			},
			want:    "feat: add feature",
			wantErr: false,
		},
		{
			name: "conditional render",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type}}{{if .Scope}}({{.Scope}}){{end}}: {{.Message}}",
				Variables: []TemplateVariable{
					{Name: "Type", Type: "string", Required: true},
					{Name: "Scope", Type: "string", Required: false},
					{Name: "Message", Type: "string", Required: true},
				},
			},
			values: map[string]string{
				"Type":    "feat",
				"Scope":   "cli",
				"Message": "add feature",
			},
			want:    "feat(cli): add feature",
			wantErr: false,
		},
		{
			name: "missing required variable",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type}}: {{.Message}}",
				Variables: []TemplateVariable{
					{Name: "Type", Type: "string", Required: true},
					{Name: "Message", Type: "string", Required: true},
				},
			},
			values: map[string]string{
				"Type": "feat",
			},
			wantErr: true,
		},
		{
			name: "use default value",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type}}: {{.Message}}",
				Variables: []TemplateVariable{
					{Name: "Type", Type: "string", Required: true, Default: "feat"},
					{Name: "Message", Type: "string", Required: true},
				},
			},
			values: map[string]string{
				"Message": "add feature",
			},
			want:    "feat: add feature",
			wantErr: false,
		},
		{
			name: "valid enum value",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type}}: {{.Message}}",
				Variables: []TemplateVariable{
					{Name: "Type", Type: "enum", Required: true, Options: []string{"feat", "fix"}},
					{Name: "Message", Type: "string", Required: true},
				},
			},
			values: map[string]string{
				"Type":    "feat",
				"Message": "add feature",
			},
			want:    "feat: add feature",
			wantErr: false,
		},
		{
			name: "invalid enum value",
			tmpl: &Template{
				Name:   "test",
				Format: "{{.Type}}: {{.Message}}",
				Variables: []TemplateVariable{
					{Name: "Type", Type: "enum", Required: true, Options: []string{"feat", "fix"}},
					{Name: "Message", Type: "string", Required: true},
				},
			},
			values: map[string]string{
				"Type":    "invalid",
				"Message": "add feature",
			},
			wantErr: true,
		},
		{
			name: "multiline with body",
			tmpl: &Template{
				Name: "test",
				Format: `{{.Type}}: {{.Description}}
{{if .Body}}
{{.Body}}
{{end}}`,
				Variables: []TemplateVariable{
					{Name: "Type", Type: "string", Required: true},
					{Name: "Description", Type: "string", Required: true},
					{Name: "Body", Type: "string", Required: false},
				},
			},
			values: map[string]string{
				"Type":        "feat",
				"Description": "add feature",
				"Body":        "This is a detailed description.",
			},
			want:    "feat: add feature\n\nThis is a detailed description.",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mgr.Render(ctx, tt.tmpl, tt.values)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Render() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Render() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTemplateManager_RenderConventional(t *testing.T) {
	ctx := context.Background()
	mgr := NewTemplateManager()

	// Load conventional template
	tmpl, err := mgr.Load(ctx, "conventional")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	tests := []struct {
		name    string
		values  map[string]string
		want    string
		wantErr bool
	}{
		{
			name: "simple commit",
			values: map[string]string{
				"Type":        "feat",
				"Description": "add commit automation",
			},
			want:    "feat: add commit automation",
			wantErr: false,
		},
		{
			name: "commit with scope",
			values: map[string]string{
				"Type":        "fix",
				"Scope":       "parser",
				"Description": "handle empty output",
			},
			want:    "fix(parser): handle empty output",
			wantErr: false,
		},
		{
			name: "commit with body",
			values: map[string]string{
				"Type":        "feat",
				"Scope":       "cli",
				"Description": "add new command",
				"Body":        "This adds a new command for automation.",
			},
			want:    "feat(cli): add new command\n\nThis adds a new command for automation.",
			wantErr: false,
		},
		{
			name: "invalid type",
			values: map[string]string{
				"Type":        "invalid",
				"Description": "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mgr.Render(ctx, tmpl, tt.values)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Render() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Render() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommitError(t *testing.T) {
	tests := []struct {
		name string
		err  *CommitError
		want string
	}{
		{
			name: "error with cause",
			err: &CommitError{
				Op:      "load",
				Message: "failed to load template",
				Cause:   ErrTemplateNotFound,
			},
			want: "load: failed to load template: template not found",
		},
		{
			name: "error without cause",
			err: &CommitError{
				Op:      "validate",
				Message: "validation failed",
			},
			want: "validate: validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
