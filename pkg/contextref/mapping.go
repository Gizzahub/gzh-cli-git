// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package contextref

import "github.com/gizzahub/gzh-cli-gitforge/pkg/cliutil"

// MapCEExit translates a CE v2 process exit and decoded envelope status
// into gz-git's exit contract. CE codes are never passed through.
func MapCEExit(ceExit int, envelopeStatus string) (gzExit int, outcome, domain string) {
	switch ceExit {
	case 0:
		if envelopeStatus != "adopted" {
			return cliutil.ExitToolError, OutcomeFault, DomainTransport
		}
		return cliutil.ExitOK, OutcomeObserved, ""
	case 1:
		if envelopeStatus == "adopted" {
			return cliutil.ExitToolError, OutcomeFault, DomainTransport
		}
		return cliutil.ExitOK, OutcomeObserved, ""
	case 2:
		return cliutil.ExitToolError, OutcomeFault, DomainCEInvocation
	default:
		return cliutil.ExitToolError, OutcomeFault, DomainTransport
	}
}
