package contextref

import "testing"

func TestParseManifestYAMLRejectsHostile(t *testing.T) {
	valid := []byte("schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: README.md\n")
	if paths, err := parseManifestYAML(valid); err != nil || len(paths) != 1 || paths[0] != "README.md" {
		t.Fatalf("valid = %v %v, want README.md", paths, err)
	}

	hostile := []struct {
		name string
		raw  string
	}{
		{"alias", "schema: gz-git.context-reference/v1\ncontext: &c\n  entrypoints: []\nx: *c\n"},
		{"duplicate key", "schema: gz-git.context-reference/v1\nschema: other\ncontext:\n  entrypoints: []\n"},
		{"unknown field", "schema: gz-git.context-reference/v1\nextra: 1\ncontext:\n  entrypoints: []\n"},
		{"multi doc", "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints: []\n---\nschema: x\n"},
		{"wrong schema", "schema: other/v1\ncontext:\n  entrypoints: []\n"},
		{"duplicate path", "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: A.md\n    - path: A.md\n"},
		{"abs path", "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: /etc/passwd\n"},
		{"int coercion", "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: 10\n"},
		{"bool coercion", "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: true\n"},
		{"float coercion", "schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: 1.5\n"},
	}
	for _, tt := range hostile {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseManifestYAML([]byte(tt.raw)); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestParseManifestSortsEntrypoints(t *testing.T) {
	raw := []byte("schema: gz-git.context-reference/v1\ncontext:\n  entrypoints:\n    - path: z.md\n    - path: a.md\n")
	paths, err := parseManifestYAML(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "a.md" || paths[1] != "z.md" {
		t.Fatalf("paths = %v, want [a.md z.md]", paths)
	}
}

func TestParseManifestLimit(t *testing.T) {
	if _, err := parseManifestYAML(make([]byte, maxManifestBytes+1)); err == nil {
		t.Fatal("expected limit error")
	}
}
