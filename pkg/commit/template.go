// Package commit provides commit message automation features.
package commit

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

//go:embed templates/*.yaml
var builtinTemplates embed.FS

// Template represents a commit message template.
type Template struct {
	Name        string             `yaml:"name"`
	Description string             `yaml:"description"`
	Format      string             `yaml:"format"`
	Variables   []TemplateVariable `yaml:"variables"`
	Rules       []ValidationRule   `yaml:"rules"`
	Examples    []string           `yaml:"examples"`
}

// TemplateVariable defines a template variable.
type TemplateVariable struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"` // string, enum, bool
	Required    bool     `yaml:"required"`
	Default     string   `yaml:"default"`
	Options     []string `yaml:"options"`     // for enum types
	Description string   `yaml:"description"`
}

// ValidationRule defines message validation rules.
type ValidationRule struct {
	Type    string `yaml:"type"`    // length, pattern, required
	Pattern string `yaml:"pattern"` // regex for pattern rules
	Message string `yaml:"message"` // error message
}

// TemplateManager manages commit message templates.
type TemplateManager interface {
	// Load loads a built-in template by name.
	Load(ctx context.Context, name string) (*Template, error)

	// LoadCustom loads a custom template from file.
	LoadCustom(ctx context.Context, path string) (*Template, error)

	// List returns available built-in template names.
	List(ctx context.Context) ([]string, error)

	// Validate validates a template.
	Validate(ctx context.Context, tmpl *Template) error

	// Render renders a template with the given values.
	Render(ctx context.Context, tmpl *Template, values map[string]string) (string, error)
}

// templateManager implements TemplateManager.
type templateManager struct{}

// NewTemplateManager creates a new TemplateManager.
func NewTemplateManager() TemplateManager {
	return &templateManager{}
}

// Load loads a built-in template by name.
func (m *templateManager) Load(ctx context.Context, name string) (*Template, error) {
	// Validate template name
	if name == "" {
		return nil, errors.New("template name cannot be empty")
	}

	// Load template file from embedded FS
	templatePath := fmt.Sprintf("templates/%s.yaml", name)
	data, err := builtinTemplates.ReadFile(templatePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrTemplateNotFound, name)
		}
		return nil, fmt.Errorf("failed to read template: %w", err)
	}

	// Parse YAML
	var tmpl Template
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}

	// Validate template
	if err := m.Validate(ctx, &tmpl); err != nil {
		return nil, err
	}

	return &tmpl, nil
}

// LoadCustom loads a custom template from file.
func (m *templateManager) LoadCustom(ctx context.Context, path string) (*Template, error) {
	// Validate path
	if path == "" {
		return nil, errors.New("template path cannot be empty")
	}

	// Clean and validate path
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		// Make relative paths absolute
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		cleanPath = filepath.Join(cwd, cleanPath)
	}

	// Read template file
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrTemplateNotFound, cleanPath)
		}
		return nil, fmt.Errorf("failed to read template: %w", err)
	}

	// Parse YAML
	var tmpl Template
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}

	// Validate template
	if err := m.Validate(ctx, &tmpl); err != nil {
		return nil, err
	}

	return &tmpl, nil
}

// List returns available built-in template names.
func (m *templateManager) List(ctx context.Context) ([]string, error) {
	entries, err := builtinTemplates.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yaml") {
			names = append(names, strings.TrimSuffix(name, ".yaml"))
		}
	}

	return names, nil
}

// Validate validates a template.
func (m *templateManager) Validate(ctx context.Context, tmpl *Template) error {
	if tmpl == nil {
		return errors.New("template cannot be nil")
	}

	// Validate required fields
	if tmpl.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidTemplate)
	}
	if tmpl.Format == "" {
		return fmt.Errorf("%w: format is required", ErrInvalidTemplate)
	}

	// Validate format is valid Go template
	_, err := template.New("test").Parse(tmpl.Format)
	if err != nil {
		return fmt.Errorf("%w: invalid template format: %v", ErrInvalidTemplate, err)
	}

	// Validate variables
	for i, v := range tmpl.Variables {
		if v.Name == "" {
			return fmt.Errorf("%w: variable %d has empty name", ErrInvalidTemplate, i)
		}
		if v.Type == "" {
			return fmt.Errorf("%w: variable %s has empty type", ErrInvalidTemplate, v.Name)
		}
		// Validate type
		switch v.Type {
		case "string", "enum", "bool":
			// Valid types
		default:
			return fmt.Errorf("%w: variable %s has invalid type: %s", ErrInvalidTemplate, v.Name, v.Type)
		}
		// Validate enum has options
		if v.Type == "enum" && len(v.Options) == 0 {
			return fmt.Errorf("%w: enum variable %s must have options", ErrInvalidTemplate, v.Name)
		}
	}

	// Validate rules
	for i, rule := range tmpl.Rules {
		if rule.Type == "" {
			return fmt.Errorf("%w: rule %d has empty type", ErrInvalidTemplate, i)
		}
		// Validate pattern rules have valid regex
		if rule.Type == "pattern" {
			if rule.Pattern == "" {
				return fmt.Errorf("%w: pattern rule %d has empty pattern", ErrInvalidTemplate, i)
			}
			if _, err := regexp.Compile(rule.Pattern); err != nil {
				return fmt.Errorf("%w: rule %d has invalid regex pattern: %v", ErrInvalidTemplate, i, err)
			}
		}
	}

	return nil
}

// Render renders a template with the given values.
func (m *templateManager) Render(ctx context.Context, tmpl *Template, values map[string]string) (string, error) {
	if tmpl == nil {
		return "", errors.New("template cannot be nil")
	}
	if values == nil {
		values = make(map[string]string)
	}

	// Validate required variables
	for _, v := range tmpl.Variables {
		if v.Required {
			val, ok := values[v.Name]
			if !ok || val == "" {
				if v.Default != "" {
					values[v.Name] = v.Default
				} else {
					return "", fmt.Errorf("required variable %s is missing", v.Name)
				}
			}
		} else if _, ok := values[v.Name]; !ok && v.Default != "" {
			// Use default for optional variables
			values[v.Name] = v.Default
		}
	}

	// Validate enum values
	for _, v := range tmpl.Variables {
		if v.Type == "enum" {
			val, ok := values[v.Name]
			if !ok {
				continue
			}
			valid := false
			for _, option := range v.Options {
				if val == option {
					valid = true
					break
				}
			}
			if !valid {
				return "", fmt.Errorf("variable %s has invalid enum value: %s (allowed: %v)", v.Name, val, v.Options)
			}
		}
	}

	// Parse and execute template
	t, err := template.New(tmpl.Name).Parse(tmpl.Format)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf strings.Builder
	if err := t.Execute(&buf, values); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	// Clean up the result
	result := strings.TrimSpace(buf.String())

	// Remove excessive blank lines (more than 2 consecutive)
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return result, nil
}
