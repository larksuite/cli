// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy

import (
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/suggest"
)

// suggestRisk returns the closest valid Risk literal by edit distance
// for risk_invalid diagnostics; input is never silently substituted.
// Case-insensitive ("WRITE" → "write"); empty in, empty out (the
// absent-annotation case goes to risk_not_annotated, not here).
func suggestRisk(bad string) string {
	if bad == "" {
		return ""
	}
	lowered := toLower(bad)
	candidates := []platform.Risk{
		platform.RiskRead, platform.RiskWrite, platform.RiskHighRiskWrite,
	}
	best := string(candidates[0])
	bestDist := suggest.Levenshtein(lowered, best)
	for _, c := range candidates[1:] {
		if d := suggest.Levenshtein(lowered, string(c)); d < bestDist {
			bestDist, best = d, string(c)
		}
	}
	return best
}

// toLower is an ASCII-only lowercase. Risk taxonomy values are
// ASCII; pulling in unicode here would be overkill.
func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
