// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package repository

import "testing"

func TestValidateForceMode(t *testing.T) {
	tests := []struct {
		value   string
		want    ForceMode
		wantErr bool
	}{
		{"", ForceModeLeaseOnly, false},
		{"lease-only", ForceModeLeaseOnly, false},
		{"allow", ForceModeAllow, false},
		{"deny", ForceModeDeny, false},
		{"yes", "", true},
		{"Allow", "", true},
	}

	for _, tt := range tests {
		got, err := ValidateForceMode(tt.value)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ValidateForceMode(%q) = %q, want error", tt.value, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ValidateForceMode(%q) returned %v", tt.value, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ValidateForceMode(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestNilPolicyPermitsEverything(t *testing.T) {
	var policy *PushPolicy

	if denial := policy.Check(PushIntent{Branch: "main", Refspec: "+main:main", Force: true}); denial != nil {
		t.Fatalf("nil policy denied a push: %+v", denial)
	}
}

func TestCheckProtectedBranch(t *testing.T) {
	policy := &PushPolicy{Protected: []string{"main", "release/*"}}

	tests := []struct {
		name   string
		intent PushIntent
		denied bool
	}{
		{"current branch is protected", PushIntent{Branch: "main"}, true},
		{"pattern matches", PushIntent{Branch: "release/2.1"}, true},
		{"feature branch is fine", PushIntent{Branch: "feat/task-001"}, false},
		{"refspec target decides, not the local branch", PushIntent{Branch: "develop", Refspec: "develop:main"}, true},
		{"local name matching is not enough", PushIntent{Branch: "develop", Refspec: "main:staging"}, false},
		{"bare refspec names its own target", PushIntent{Branch: "develop", Refspec: "main"}, true},
		{"full ref path resolves to the branch", PushIntent{Branch: "develop", Refspec: "develop:refs/heads/main"}, true},
		{"detached head matches nothing", PushIntent{Branch: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			denial := policy.Check(tt.intent)
			if tt.denied {
				if denial == nil {
					t.Fatal("expected a denial")
				}
				if denial.Rule != PushRuleProtected {
					t.Errorf("rule = %q, want %q", denial.Rule, PushRuleProtected)
				}
				return
			}
			if denial != nil {
				t.Fatalf("unexpected denial: %+v", denial)
			}
		})
	}
}

func TestLeaseOnlyRefusesRawForceButAllowsLease(t *testing.T) {
	policy := &PushPolicy{}

	denial := policy.Check(PushIntent{Branch: "develop", Refspec: "+develop:develop"})
	if denial == nil {
		t.Fatal("expected a raw force to be denied")
	}
	if denial.Rule != PushRuleRawForce {
		t.Errorf("rule = %q, want %q", denial.Rule, PushRuleRawForce)
	}

	if denial := policy.Check(PushIntent{Branch: "develop", Force: true}); denial != nil {
		t.Fatalf("--force should be allowed under lease-only: %+v", denial)
	}
}

func TestForceModeAllowPermitsRawForce(t *testing.T) {
	policy := &PushPolicy{ForceMode: ForceModeAllow}

	if denial := policy.Check(PushIntent{Branch: "develop", Refspec: "+develop:develop"}); denial != nil {
		t.Fatalf("unexpected denial: %+v", denial)
	}
}

func TestForceModeAllowStillHonorsProtection(t *testing.T) {
	policy := &PushPolicy{Protected: []string{"main"}, ForceMode: ForceModeAllow}

	denial := policy.Check(PushIntent{Branch: "main"})
	if denial == nil || denial.Rule != PushRuleProtected {
		t.Fatalf("expected a protected-branch denial, got %+v", denial)
	}
}

func TestForceModeDenyRefusesBothKindsOfForce(t *testing.T) {
	policy := &PushPolicy{ForceMode: ForceModeDeny}

	for _, intent := range []PushIntent{
		{Branch: "develop", Force: true},
		{Branch: "develop", Refspec: "+develop:develop"},
	} {
		denial := policy.Check(intent)
		if denial == nil {
			t.Fatalf("expected a denial for %+v", intent)
		}
		if denial.Rule != PushRuleForceDenied {
			t.Errorf("rule = %q, want %q", denial.Rule, PushRuleForceDenied)
		}
	}

	if denial := policy.Check(PushIntent{Branch: "develop"}); denial != nil {
		t.Fatalf("a plain push should still be allowed: %+v", denial)
	}
}

func TestCheckIgnoresUnparseableRefspec(t *testing.T) {
	// pushRepository reports the parse error itself; the policy must not turn
	// a malformed refspec into a policy violation.
	policy := &PushPolicy{Protected: []string{"main"}}

	if denial := policy.Check(PushIntent{Branch: "main", Refspec: "a:b:c"}); denial != nil {
		t.Fatalf("unexpected denial: %+v", denial)
	}
}
