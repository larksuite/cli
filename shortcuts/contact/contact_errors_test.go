// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestContactFanoutErrorSummary_HTTPStatus(t *testing.T) {
	err := errs.NewNetworkError(errs.SubtypeNetworkServer, `HTTP 503: {"reason":"upstream_unavailable"}`).
		WithCode(503).
		WithRetryable()

	got := contactFanoutErrorSummary(err)
	if !strings.HasPrefix(got, "HTTP 503 Service Unavailable: ") {
		t.Fatalf("summary: got %q", got)
	}
	if !strings.Contains(got, "upstream_unavailable") {
		t.Fatalf("summary should include truncated body details, got %q", got)
	}
}

func TestContactInvalidResponseError_TypedInternal(t *testing.T) {
	got := contactInvalidResponseError("decode contact response failed")
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed problem, got %T", got)
	}
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem type: got %s/%s", p.Category, p.Subtype)
	}
}

func TestContactFanoutAllFailedError_PreservesTypedProblem(t *testing.T) {
	err := errs.NewAPIError(errs.SubtypeRateLimit, "rate limit").
		WithCode(99991663).
		WithLogID("log-contact-1").
		WithRetryable()

	got := contactFanoutAllFailedError(err, "all 2 queries failed; first: API 99991663: rate limit (query=\"alice\")")
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed problem, got %T", got)
	}
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeRateLimit {
		t.Fatalf("problem type: got %s/%s", p.Category, p.Subtype)
	}
	if p.Code != 99991663 || p.LogID != "log-contact-1" || !p.Retryable {
		t.Fatalf("problem metadata not preserved: %+v", p)
	}
	if !strings.Contains(p.Message, "all 2 queries failed") {
		t.Fatalf("problem message not decorated: %q", p.Message)
	}
	// The representative error must not be mutated: it stays a single-query
	// failure, while the aggregate is a distinct value carrying it as cause.
	if err.Message != "rate limit" {
		t.Fatalf("representative error message was mutated: %q", err.Message)
	}
	if !errors.Is(got, err) {
		t.Fatalf("aggregate error should keep the representative failure as its cause")
	}
}

func TestContactFanoutAllFailedError_UntypedGetsActionableHint(t *testing.T) {
	got := contactFanoutAllFailedError(nil, "all 2 queries failed; first: internal error (query=\"alice\")")
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed problem, got %T", got)
	}
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeUnknown {
		t.Fatalf("problem type: got %s/%s", p.Category, p.Subtype)
	}
	if !strings.Contains(p.Hint, "narrow --queries") {
		t.Fatalf("hint should guide recovery, got %q", p.Hint)
	}
}
