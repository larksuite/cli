// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"strings"
	"testing"
)

func TestSkippedDecisionUsesSystemWarning(t *testing.T) {
	got := SkippedDecision(ErrReviewerUnavailable)
	if !got.Skipped || got.Degraded || got.InfrastructureFailure {
		t.Fatalf("unexpected skipped decision: %#v", got)
	}
	if len(got.SystemWarnings) != 1 || got.SystemWarnings[0].Severity != "minor" {
		t.Fatalf("missing system warning: %#v", got)
	}
	if len(got.Warnings) != 0 || len(got.Blockers) != 0 {
		t.Fatalf("skipped decision must not fake finding evidence: %#v", got)
	}
}

func TestDegradedDecisionUsesSystemWarning(t *testing.T) {
	got := DegradedDecision(ErrReviewerUnavailable)
	if !got.Degraded || got.Skipped || got.InfrastructureFailure {
		t.Fatalf("unexpected degraded decision: %#v", got)
	}
	if len(got.SystemWarnings) != 1 {
		t.Fatalf("missing system warning: %#v", got)
	}
	if len(got.Warnings) != 0 || len(got.Blockers) != 0 {
		t.Fatalf("degraded decision must not fake finding evidence: %#v", got)
	}
}

func TestMarkdownSanitizesFindingMessages(t *testing.T) {
	got := Markdown(Decision{Blockers: []Finding{{
		Message: "@team\n# forged [link](https://example.com)<b>",
	}}})
	if strings.Contains(got, "@team") || strings.Contains(got, "\n# forged") || strings.Contains(got, "<b>") || strings.Contains(got, "https://example.com") {
		t.Fatalf("finding message was not sanitized:\n%s", got)
	}
	for _, want := range []string{"@\u200bteam", "\\# forged", "\\[link\\]", "https[:]//example.com", "&lt;b&gt;"} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitized markdown missing %q:\n%s", want, got)
		}
	}
}
