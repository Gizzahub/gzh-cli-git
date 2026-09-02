// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package config

import "testing"

const validReadinessDoc = "branch:\n  readiness:\n    version: 1\n    runner: .gz-git/readiness/run.sh\n"

// TestParseReadinessDocumentMultiDocumentIsRejected pins the reader against a
// config whose first document parses cleanly and whose later documents carry
// the payload. yaml.Unmarshal returned only document 1, so every field
// allowlist in this file inspected a contract the file did not solely contain.
func TestParseReadinessDocumentMultiDocumentIsRejected(t *testing.T) {
	rejected := map[string]string{
		"second document declares hooks":  validReadinessDoc + "---\nhooks:\n  evil: ./x\n",
		"second document declares branch": validReadinessDoc + "---\nbranch:\n  taskPattern:\n    - \"\"\n",
		"second document is empty":        validReadinessDoc + "---\n",
		"three documents":                 validReadinessDoc + "---\na: 1\n---\nb: 2\n",
	}
	for name, in := range rejected {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ParseReadinessDocument([]byte(in), false); err == nil {
				t.Fatalf("multi-document config accepted; the payload after document 1 was never read")
			}
		})
	}

	// Over-rejection would be its own defect: a leading marker opens the first
	// document rather than adding a second, and is ordinary YAML.
	accepted := map[string]string{
		"single document":               validReadinessDoc,
		"single document with marker":   "---\n" + validReadinessDoc,
		"single document, no readiness": "branch:\n  integrationBranch: master\n",
	}
	for name, in := range accepted {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ParseReadinessDocument([]byte(in), false); err != nil {
				t.Fatalf("single-document config rejected: %v", err)
			}
		})
	}
}
