// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestNormalizeHTTPError(t *testing.T) {
	original := errs.NewAPIError(errs.SubtypeUnknown, "business error").WithCode(123)
	got := NormalizeHTTPError(503, "log-id", original)
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Category != errs.CategoryNetwork ||
		problem.Subtype != errs.SubtypeNetworkServer ||
		problem.Code != 503 || problem.LogID != "log-id" || !problem.Retryable {
		t.Fatalf("normalized problem = %#v, err=%T %v", problem, got, got)
	}

	rateLimited := NormalizeHTTPError(429, "rate-log", nil)
	rateProblem, ok := errs.ProblemOf(rateLimited)
	if !ok || rateProblem.Category != errs.CategoryAPI ||
		rateProblem.Subtype != errs.SubtypeRateLimit ||
		rateProblem.Code != 429 || rateProblem.LogID != "rate-log" || rateProblem.Retryable {
		t.Fatalf("rate-limit problem = %#v, err=%T %v", rateProblem, rateLimited, rateLimited)
	}

	notFound := NormalizeHTTPError(404, "", nil)
	notFoundProblem, ok := errs.ProblemOf(notFound)
	if !ok || notFoundProblem.Subtype != errs.SubtypeNotFound ||
		notFoundProblem.Code != 404 || notFoundProblem.Retryable {
		t.Fatalf("not-found problem = %#v, err=%T %v", notFoundProblem, notFound, notFound)
	}

	if unchanged := NormalizeHTTPError(200, "", original); unchanged != original {
		t.Fatalf("successful status was normalized: %T %v", unchanged, unchanged)
	}
}
