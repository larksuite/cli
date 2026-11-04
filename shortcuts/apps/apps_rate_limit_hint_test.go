// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
)

// appsGatewayRateLimitBody is the OpenAPI gateway throttling envelope. 99991400 is
// registered in internal/errclass as api/rate_limit, retryable.
func appsGatewayRateLimitBody() map[string]interface{} {
	return map[string]interface{}{
		"code": 99991400,
		"msg":  "request trigger frequency limit",
		"error": map[string]interface{}{
			"log_id": "log-placeholder",
		},
	}
}

// A rate-limited +db-execute was answered with "verify table/column names ...
// target the dev database with --environment dev": advice about the request, for
// a failure that says nothing about the request. An agent following it sends more
// requests into the same limit. The hint must point at backing off instead.
func TestAppsDBExecute_RateLimitHintIsBackoffNotRequestFix(t *testing.T) {
	for _, status := range []int{200, 429} {
		t.Run(rateLimitStatusName(status), func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
				Status: status,
				Body:   appsGatewayRateLimitBody(),
			})
			err := runAppsShortcut(t, AppsDBExecute,
				[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "select 1", "--as", "user"},
				factory, stdout)
			p := requireAppsAPIProblem(t, err)
			raw, _ := json.Marshal(err)
			t.Logf("error envelope: %s", raw)

			// Classification is already right; only the hint is wrong.
			if p.Subtype != errs.SubtypeRateLimit || p.Code != 99991400 || !p.Retryable {
				t.Fatalf("subtype/code/retryable = %q/%d/%t, want rate_limit/99991400/true", p.Subtype, p.Code, p.Retryable)
			}
			for _, requestAdvice := range []string{"+db-table-get", "--environment dev", "table/column"} {
				if strings.Contains(p.Hint, requestAdvice) {
					t.Errorf("rate-limited +db-execute still tells the caller to change the request (%q): hint=%q", requestAdvice, p.Hint)
				}
			}
			if !strings.Contains(p.Hint, "backoff") {
				t.Errorf("hint = %q, want backoff guidance for a throttled request", p.Hint)
			}
		})
	}
}

// The apps commands that attach a request-shaped hint do it through withAppsHint
// (79 sites), withObservabilityHint (2) or withDBSyncHint (6). Drive a real
// 99991400 classification through each helper with the request-shaped hints the
// hottest commands pass, so the fix is proven at the chokepoint and not per
// command. Commands that return the client error as is (+list, for one) keep the
// classifier's rate_limit with an empty hint, which misleads no one; commands
// with their own error helpers are outside this test.
func TestAppsHints_RateLimitDoesNotInheritRequestHint(t *testing.T) {
	dbExecuteHint := "verify table/column names with `lark-cli apps +db-table-get --app-id app_x --table <table>`; for day-to-day debugging target the dev database with `--environment dev`"
	for _, tc := range []struct {
		name        string
		requestHint string
		apply       func(error) error
	}{
		{"+trace-get (appIDListHint)", appIDListHint, func(err error) error { return withAppsHint(err, appIDListHint) }},
		{"+db-execute", dbExecuteHint, func(err error) error { return withAppsHint(err, dbExecuteHint) }},
		{"+session-messages-list", sessionMessagesListHint, func(err error) error { return withAppsHint(err, sessionMessagesListHint) }},
		{"+metric-list / +analytics-list (withObservabilityHint)", appIDListHint, withObservabilityHint},
		{"+db-sync-list (withDBSyncHint)", dbSyncListHint, func(err error) error { return withDBSyncHint(err, dbSyncListHint) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := errclass.BuildAPIError(appsGatewayRateLimitBody(), errclass.ClassifyContext{Identity: "user"})
			out := tc.apply(in)
			if out != in {
				t.Fatalf("helper returned a different error value; the cause chain must survive")
			}
			p, ok := errs.ProblemOf(out)
			if !ok {
				t.Fatalf("not a typed problem: %T", out)
			}
			t.Logf("hint after helper: %q", p.Hint)
			if p.Subtype != errs.SubtypeRateLimit || p.Code != 99991400 || !p.Retryable {
				t.Fatalf("classification mutated: subtype=%q code=%d retryable=%t", p.Subtype, p.Code, p.Retryable)
			}
			if p.Hint == tc.requestHint {
				t.Errorf("rate limit inherited the request-shaped hint %q", p.Hint)
			}
			if !strings.Contains(p.Hint, "backoff") {
				t.Errorf("hint = %q, want backoff guidance", p.Hint)
			}
		})
	}
}

// The rate-limit branch only fills an empty hint: wording the upstream sent (a
// lifted error.details value, or the stream client's retry-after sentence) wins,
// exactly as it does for every other class.
func TestWithAppsHint_RateLimitKeepsUpstreamHint(t *testing.T) {
	cause := errors.New("upstream cause")
	in := errs.NewAPIError(errs.SubtypeRateLimit, "HTTP 429: throttled").WithCode(429).WithRetryable().
		WithHint("wait at least 3 seconds before retrying; use exponential backoff with jitter if throttling continues").
		WithCause(cause)
	out := withAppsHint(in, appIDListHint)
	p, _ := errs.ProblemOf(out)
	if p.Hint != "wait at least 3 seconds before retrying; use exponential backoff with jitter if throttling continues" {
		t.Fatalf("hint = %q, want the upstream retry-after hint preserved", p.Hint)
	}
	if !errors.Is(out, cause) {
		t.Fatalf("cause chain dropped")
	}
}

// Rate-limit classification is stronger evidence than message text. The same
// helpers also recognize database state from message markers, but a gateway
// throttle can carry arbitrary upstream wording and was not evaluated by the
// database service. Those markers must not turn a throttle into request advice.
func TestWithAppsHint_RateLimitWinsOverDatabaseMessageMarkers(t *testing.T) {
	for _, message := range []string{
		"k_dl_4000004: request trigger frequency limit",
		"no db branch: request trigger frequency limit",
	} {
		t.Run(message, func(t *testing.T) {
			in := errs.NewAPIError(errs.SubtypeRateLimit, "%s", message).
				WithCode(99991400).WithRetryable()
			out := withAppsHint(in, appIDListHint)
			p, _ := errs.ProblemOf(out)
			if p.Subtype != errs.SubtypeRateLimit || p.Code != 99991400 || !p.Retryable {
				t.Fatalf("classification mutated: subtype=%q code=%d retryable=%t", p.Subtype, p.Code, p.Retryable)
			}
			if p.Message != message {
				t.Fatalf("message = %q, want original %q", p.Message, message)
			}
			if p.Hint != appRateLimitHint {
				t.Fatalf("hint = %q, want rate-limit backoff hint %q", p.Hint, appRateLimitHint)
			}
		})
	}
}

// The observability wrapper has its own message heuristic before withAppsHint.
// A throttle whose upstream text resembles the no-container business error must
// still keep its classification and tell the caller to back off, not deploy.
func TestWithObservabilityHint_RateLimitWinsOverContainerMessageMarker(t *testing.T) {
	message := "container not exist: request trigger frequency limit"
	in := errs.NewAPIError(errs.SubtypeRateLimit, "%s", message).
		WithCode(99991400).WithRetryable()
	out := withObservabilityHint(in)
	p, _ := errs.ProblemOf(out)
	if p.Subtype != errs.SubtypeRateLimit || p.Code != 99991400 || !p.Retryable {
		t.Fatalf("classification mutated: subtype=%q code=%d retryable=%t", p.Subtype, p.Code, p.Retryable)
	}
	if p.Message != message {
		t.Fatalf("message = %q, want original %q", p.Message, message)
	}
	if p.Hint != appRateLimitHint {
		t.Fatalf("hint = %q, want rate-limit backoff hint %q", p.Hint, appRateLimitHint)
	}
}

func rateLimitStatusName(status int) string {
	if status == 200 {
		return "HTTP 200 envelope"
	}
	return "HTTP 429 envelope"
}
