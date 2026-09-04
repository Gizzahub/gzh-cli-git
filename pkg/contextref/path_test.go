package contextref

import "testing"

func TestValidateEntrypointPath(t *testing.T) {
	ok := []string{"README.md", "docs/SOUL.md", "a/b/c.txt"}
	for _, p := range ok {
		if err := validateEntrypointPath(p); err != nil {
			t.Errorf("validateEntrypointPath(%q) = %v, want nil", p, err)
		}
	}
	bad := []string{
		"", "/abs", "dir/", `win\path`, "a/../b", ".", "..", "foo/./bar",
		"C:docs", "//unc/share", "CON.md", "aux.txt", "foo ", "foo.",
		"a/\x00b", "a/\tb",
	}
	for _, p := range bad {
		if err := validateEntrypointPath(p); err == nil {
			t.Errorf("validateEntrypointPath(%q) = nil, want error", p)
		}
	}
}
