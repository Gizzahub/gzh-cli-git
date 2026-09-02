// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package integrate

import (
	"strings"
	"testing"
)

const (
	targetContract = "branch:\n  integrationBranch: master\n"
	sourceContract = "branch:\n  integrationBranch: master\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/run.sh\n"
)

// TestOnlyReadinessAdditionMultiDocumentIsRejected covers the existing-contract
// path. equalJSON compares document 1 of each side, so a second document was
// never part of the "nothing changed beyond branch.readiness" claim: it landed
// on a protected branch under a check that had not read it.
func TestOnlyReadinessAdditionMultiDocumentIsRejected(t *testing.T) {
	t.Run("payload hidden in the source", func(t *testing.T) {
		neu := sourceContract + "---\nhooks:\n  evil: ./x\n"
		err := onlyReadinessAddition([]byte(targetContract), []byte(neu))
		if err == nil {
			t.Fatal("source with a second document accepted as a readiness-only addition")
		}
		if !strings.Contains(err.Error(), "source") {
			t.Fatalf("error does not name the offending side: %v", err)
		}
	})

	t.Run("second document in the target", func(t *testing.T) {
		old := targetContract + "---\nbranch:\n  taskPattern:\n    - \"\"\n"
		err := onlyReadinessAddition([]byte(old), []byte(sourceContract))
		if err == nil {
			t.Fatal("target with a second document accepted; 'unchanged' was claimed about a file only partly read")
		}
		if !strings.Contains(err.Error(), "target") {
			t.Fatalf("error does not name the offending side: %v", err)
		}
	})

	t.Run("single documents still accepted", func(t *testing.T) {
		if err := onlyReadinessAddition([]byte(targetContract), []byte(sourceContract)); err != nil {
			t.Fatalf("ordinary readiness addition rejected: %v", err)
		}
	})
}

// TestNewContractIsMinimalMultiDocumentIsRejected covers the other dispatch
// arm. It needs no guard of its own — it delegates to the same reader — so this
// test exists to keep that delegation from being replaced by a local shape
// check that would reopen the hole.
func TestNewContractIsMinimalMultiDocumentIsRejected(t *testing.T) {
	neu := "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/run.sh\n---\nhooks:\n  evil: ./x\n"
	if err := newContractIsMinimal([]byte(neu)); err == nil {
		t.Fatal("new contract with a second document accepted")
	}
	single := "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/run.sh\n"
	if err := newContractIsMinimal([]byte(single)); err != nil {
		t.Fatalf("ordinary new contract rejected: %v", err)
	}
}
