// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"strings"
	"testing"

	"github.com/gizzahub/gzh-cli-gitforge/pkg/identity"
)

func TestParseKind(t *testing.T) {
	cases := []struct {
		value   string
		want    Kind
		wantErr bool
	}{
		{"", KindWork, false},
		{"work", KindWork, false},
		{"device", KindDevice, false},
		{"agent", KindAgent, false},
		{"Device", "", true},
		{"machine", "", true},
	}

	for _, tc := range cases {
		got, err := ParseKind(tc.value)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseKind(%q) = %q, want an error", tc.value, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseKind(%q) returned %v", tc.value, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseKind(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestResolveUsesTheDocumentedConvention(t *testing.T) {
	id := identity.Identity{Device: "dave-office", Agent: "hermes-01"}

	cases := []struct {
		kind Kind
		want string
	}{
		{KindWork, "feat/task-001-product-unit"},
		{KindDevice, "feat/task-001-product-unit/dave-office"},
		{KindAgent, "agent/task-001-product-unit/hermes-01"},
	}

	for _, tc := range cases {
		got, err := (*Naming)(nil).Resolve(tc.kind, "task-001-product-unit", id)
		if err != nil {
			t.Errorf("Resolve(%q) returned %v", tc.kind, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestConfiguredTemplateOverridesOnlyItsOwnKind(t *testing.T) {
	naming := &Naming{Device: "wip/{device}/{task}"}
	id := identity.Identity{Device: "dave-office"}

	got, err := naming.Resolve(KindDevice, "task-002", id)
	if err != nil {
		t.Fatalf("Resolve returned %v", err)
	}
	if want := "wip/dave-office/task-002"; got != want {
		t.Errorf("device = %q, want %q", got, want)
	}

	// The other two kinds were not configured and keep their defaults.
	if got, err := naming.Resolve(KindWork, "task-002", id); err != nil || got != "feat/task-002" {
		t.Errorf("work = %q, %v; want feat/task-002", got, err)
	}
}

func TestResolveSlugifiesAHostnameThatIsNotABranchName(t *testing.T) {
	// os.Hostname() on macOS routinely returns something like this, and it is
	// the default device name, so it has to survive without configuration.
	id := identity.Identity{Device: "Daves-MacBook.local"}

	got, err := (*Naming)(nil).Resolve(KindDevice, "Task 003: Product Unit", id)
	if err != nil {
		t.Fatalf("Resolve returned %v", err)
	}
	if want := "feat/task-003-product-unit/daves-macbook-local"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveNeedsTheIdentityItsKindNames(t *testing.T) {
	// A device branch with no device name is the shared branch again under a
	// longer name — the exact collision splitting the branch is meant to avoid.
	if _, err := (*Naming)(nil).Resolve(KindDevice, "task-004", identity.Identity{}); err == nil {
		t.Error("a device branch without a device name should be refused")
	}

	if _, err := (*Naming)(nil).Resolve(KindAgent, "task-004", identity.Identity{Device: "dave-office"}); err == nil {
		t.Error("an agent branch without an agent name should be refused")
	}

	// A work branch names no writer, so it needs no identity at all.
	if _, err := (*Naming)(nil).Resolve(KindWork, "task-004", identity.Identity{}); err != nil {
		t.Errorf("work branch returned %v, want it to resolve without an identity", err)
	}
}

func TestResolveRejectsATaskWithNothingUsableInIt(t *testing.T) {
	if _, err := (*Naming)(nil).Resolve(KindWork, "///", identity.Identity{}); err == nil {
		t.Error("expected an error for a task that slugifies to nothing")
	}
}

func TestResolveRejectsAMisspelledPlaceholder(t *testing.T) {
	// Substitution leaves an unknown placeholder intact rather than emptying it,
	// so the braces reach validation and the typo is reported instead of baked
	// into a branch name.
	naming := &Naming{Work: "feat/{tsak}"}

	_, err := naming.Resolve(KindWork, "task-005", identity.Identity{})
	if err == nil {
		t.Fatal("expected an error for the misspelled placeholder")
	}
	if !strings.Contains(err.Error(), "{tsak}") {
		t.Errorf("error %q should name the offending template", err)
	}
}

func TestTemplateFallsBackPerKind(t *testing.T) {
	cases := []struct {
		kind Kind
		want string
	}{
		{KindWork, DefaultWorkTemplate},
		{KindDevice, DefaultDeviceTemplate},
		{KindAgent, DefaultAgentTemplate},
	}

	for _, tc := range cases {
		got, err := (&Naming{}).Template(tc.kind)
		if err != nil {
			t.Errorf("Template(%q) returned %v", tc.kind, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Template(%q) = %q, want %q", tc.kind, got, tc.want)
		}
	}

	if _, err := (*Naming)(nil).Template(Kind("nonsense")); err == nil {
		t.Error("expected an error for an unknown kind")
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"task-001":            "task-001",
		"Task 001":            "task-001",
		"  spaced  out  ":     "spaced-out",
		"dots.and_slashes/x":  "dots-and-slashes-x",
		"UPPER":               "upper",
		"---":                 "",
		"feat/TASK-9: thing!": "feat-task-9-thing",
	}

	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
