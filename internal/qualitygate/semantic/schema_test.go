// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"strings"
	"testing"
)

func TestDecodeReviewRejectsUnknownFieldsAndBlocking(t *testing.T) {
	raw := `{"verdict":"warn","blocking":true,"findings":[]}`
	_, err := DecodeReview(strings.NewReader(raw))
	if err == nil {
		t.Fatal("DecodeReview accepted unknown blocking field")
	}
}

func TestDecodeReviewRejectsTooManyFindings(t *testing.T) {
	raw := `{"verdict":"warn","findings":[`
	for i := 0; i < 21; i++ {
		if i > 0 {
			raw += ","
		}
		raw += `{"category":"skill_quality","severity":"minor","evidence":["facts.skills[0]"],"message":"m","suggested_action":"a"}`
	}
	raw += `]}`
	_, err := DecodeReview(strings.NewReader(raw))
	if err == nil {
		t.Fatal("DecodeReview accepted too many findings")
	}
}

func TestDecodeModelReviewAcceptsStringEvidence(t *testing.T) {
	raw := `{"verdict":"warn","findings":[{"category":"error_hint","severity":"major","evidence":"facts.errors[0]","message":"hint is vague","suggested_action":"include a concrete command or flag"}]}`
	review, err := DecodeModelReview(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("DecodeModelReview() error = %v", err)
	}
	if len(review.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(review.Findings))
	}
	if got := review.Findings[0].Evidence; len(got) != 1 || got[0] != "facts.errors[0]" {
		t.Fatalf("evidence = %#v, want single fact ref", got)
	}
	if _, err := DecodeReview(strings.NewReader(raw)); err == nil {
		t.Fatal("DecodeReview accepted model-only string evidence")
	}
}
