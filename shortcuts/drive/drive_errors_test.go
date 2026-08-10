// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestWithDriveRateLimitRecoveryHintPreservesTypedError(t *testing.T) {
	cause := errors.New("rate-limit sentinel")
	err := errs.NewAPIError(errs.SubtypeRateLimit, "request trigger frequency limit").
		WithCode(99991400).
		WithLogID("log-drive-rate-limit").
		WithRetryable().
		WithHint("upstream hint").
		WithCause(cause)

	got := withRateLimitRecoveryHint(err)
	if got != err {
		t.Fatalf("decorated error identity changed: got %T, want original %T", got, err)
	}
	if !errors.Is(got, cause) {
		t.Fatal("decorated error lost its cause")
	}
	problem, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", got, got)
	}
	if problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeRateLimit || problem.Code != 99991400 || problem.LogID != "log-drive-rate-limit" || !problem.Retryable {
		t.Fatalf("problem=%+v, want preserved rate-limit metadata", problem)
	}
	for _, want := range []string{"upstream hint", "stop immediate retries", "exponential backoff", "jitter"} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint=%q, want %q", problem.Hint, want)
		}
	}
}

func TestWithDriveRateLimitRecoveryHintHandlesHTTP429AndAvoidsDuplicates(t *testing.T) {
	err := errs.NewNetworkError(errs.SubtypeNetworkTransport, "HTTP 429").
		WithCode(http.StatusTooManyRequests)

	got := withRateLimitRecoveryHint(err)
	got = withRateLimitRecoveryHint(got)
	problem, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", got, got)
	}
	if count := strings.Count(problem.Hint, rateLimitRecoveryHint); count != 1 {
		t.Fatalf("hint=%q, generic recovery count=%d, want 1", problem.Hint, count)
	}
}

func TestWithDriveRateLimitRecoveryHintKeepsDailyQuotaGuidance(t *testing.T) {
	const dailyHint = "wait for the daily quota to reset before retrying"
	err := errs.NewAPIError(errs.SubtypeRateLimit, "permission-apply quota reached").
		WithCode(1063006).
		WithHint(dailyHint)

	got := withRateLimitRecoveryHint(err)
	problem, _ := errs.ProblemOf(got)
	if problem.Hint != dailyHint {
		t.Fatalf("hint=%q, want daily-quota guidance unchanged", problem.Hint)
	}
}

func TestDriveClassifyBatchFailureIncludesRateLimitHint(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantRetryable bool
	}{
		{
			name:          "business code",
			err:           errs.NewAPIError(errs.SubtypeRateLimit, "batch item throttled").WithCode(99991400).WithRetryable(),
			wantRetryable: true,
		},
		{
			name: "HTTP 429",
			err:  errs.NewNetworkError(errs.SubtypeNetworkTransport, "batch item throttled").WithCode(http.StatusTooManyRequests),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := driveClassifyBatchFailure(tt.err)
			if decision.Class != "rate_limited" || !decision.Terminal || decision.Retryable != tt.wantRetryable {
				t.Fatalf("decision=%+v, want terminal rate_limited retryable=%v", decision, tt.wantRetryable)
			}
			if !strings.Contains(decision.Hint, "exponential backoff") {
				t.Fatalf("hint=%q, want exponential-backoff guidance", decision.Hint)
			}
		})
	}
}
