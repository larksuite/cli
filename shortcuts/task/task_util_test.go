// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/smartystreets/goconvey/convey"
)

func TestContains(t *testing.T) {
	convey.Convey("contains", t, func() {
		list := []string{"a", "b", "c"}
		convey.So(contains(list, "a"), convey.ShouldBeTrue)
		convey.So(contains(list, "d"), convey.ShouldBeFalse)
		convey.So(contains([]string{}, "a"), convey.ShouldBeFalse)
	})
}

func TestParseRelativeTime_TypedError(t *testing.T) {
	_, err := parseRelativeTime("not-relative")
	if err == nil {
		t.Fatal("parseRelativeTime(\"not-relative\") expected error, got nil")
	}

	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *errs.ValidationError; error = %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", got, output.ExitValidation)
	}
	if !strings.Contains(err.Error(), "invalid relative time format") {
		t.Errorf("message = %q, want substring %q", err.Error(), "invalid relative time format")
	}
}

// apiResult builds a parsed Lark API response with a non-zero code, as
// HandleTaskApiResult receives it after json.Unmarshal.
func apiResult(code int, msg string) map[string]interface{} {
	return map[string]interface{}{"code": float64(code), "msg": msg}
}

// TestHandleTaskApiResult_TypedMapping locks the API code → typed
// category/subtype/exit mapping. Classification is sourced from
// internal/errclass/codemeta_task.go via errclass.BuildAPIError; the
// task-specific recovery hint is layered on from taskAPIHints. 1470400 surfaces
// exit 1 (API-side parameter rejection, was exit 2 under legacy); 1470403
// routes to CategoryAuthorization and surfaces exit 3 (was exit 1).
func TestHandleTaskApiResult_TypedMapping(t *testing.T) {
	tests := []struct {
		name        string
		code        int
		wantSubtype errs.Subtype
		wantExit    int
		wantRetry   bool
	}{
		{"invalid_params", ErrCodeTaskInvalidParams, errs.SubtypeInvalidParameters, output.ExitAPI, false},
		{"not_found", ErrCodeTaskNotFound, errs.SubtypeNotFound, output.ExitAPI, false},
		{"conflict", ErrCodeTaskConflict, errs.SubtypeConflict, output.ExitAPI, true},
		{"internal", ErrCodeTaskInternalError, errs.SubtypeServerError, output.ExitAPI, true},
		{"assignee_limit", ErrCodeTaskAssigneeLimit, errs.SubtypeQuotaExceeded, output.ExitAPI, false},
		{"follower_limit", ErrCodeTaskFollowerLimit, errs.SubtypeQuotaExceeded, output.ExitAPI, false},
		{"member_limit", ErrCodeTasklistMemberLimit, errs.SubtypeQuotaExceeded, output.ExitAPI, false},
		{"reminder_exists", ErrCodeTaskReminderExists, errs.SubtypeAlreadyExists, output.ExitAPI, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := HandleTaskApiResult(apiResult(tt.code, "raw upstream detail"), nil, "do thing")
			if data != nil {
				t.Errorf("data = %v, want nil on error", data)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("err = %T, want typed errs.* error", err)
			}
			if p.Subtype != tt.wantSubtype {
				t.Errorf("subtype = %q, want %q", p.Subtype, tt.wantSubtype)
			}
			if p.Code != tt.code {
				t.Errorf("code = %d, want %d", p.Code, tt.code)
			}
			if got := output.ExitCodeOf(err); got != tt.wantExit {
				t.Errorf("exit code = %d, want %d", got, tt.wantExit)
			}
			if p.Retryable != tt.wantRetry {
				t.Errorf("retryable = %v, want %v", p.Retryable, tt.wantRetry)
			}
			// These CategoryAPI codes carry the task-specific recovery hint.
			if p.Hint != taskAPIHints[tt.code] {
				t.Errorf("hint = %q, want %q", p.Hint, taskAPIHints[tt.code])
			}
			// BuildAPIError uses the raw upstream msg as the message.
			if !strings.Contains(err.Error(), "raw upstream detail") {
				t.Errorf("message = %q, want raw upstream detail", err.Error())
			}
		})
	}
}

// TestHandleTaskApiResult_PermissionDenied verifies 1470403 routes to a typed
// *errs.PermissionError (exit 3) carrying the canonical permission hint from
// BuildAPIError — taskAPIHints intentionally omits it so the canonical hint
// stands.
func TestHandleTaskApiResult_PermissionDenied(t *testing.T) {
	_, err := HandleTaskApiResult(apiResult(ErrCodeTaskPermissionDenied, "no permission"), nil, "do thing")
	var pe *errs.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %T, want *errs.PermissionError", err)
	}
	if pe.Subtype != errs.SubtypePermissionDenied {
		t.Errorf("subtype = %q, want %q", pe.Subtype, errs.SubtypePermissionDenied)
	}
	if got := output.ExitCodeOf(err); got != output.ExitAuth {
		t.Errorf("exit code = %d, want %d", got, output.ExitAuth)
	}
	if strings.TrimSpace(pe.Hint) == "" {
		t.Error("expected a canonical permission hint, got empty")
	}
}

func TestHandleTaskApiResultWithContext_PermissionConsoleURL(t *testing.T) {
	_, err := HandleTaskApiResultWithContext(map[string]interface{}{
		"code": float64(99991672),
		"msg":  "access denied",
		"error": map[string]interface{}{
			"permission_violations": []interface{}{
				map[string]interface{}{"subject": "task:attachment:write"},
			},
		},
	}, nil, "upload task attachment", errclass.ClassifyContext{
		Brand:    "lark",
		AppID:    "cli_a123",
		Identity: "bot",
	})

	var pe *errs.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %T, want *errs.PermissionError", err)
	}
	if pe.Subtype != errs.SubtypeAppScopeNotApplied {
		t.Errorf("subtype = %q, want %q", pe.Subtype, errs.SubtypeAppScopeNotApplied)
	}
	if pe.ConsoleURL == "" || !strings.Contains(pe.ConsoleURL, "open.larksuite.com/app/cli_a123/auth") {
		t.Errorf("ConsoleURL = %q, want Lark developer console URL", pe.ConsoleURL)
	}
	if len(pe.MissingScopes) != 1 || pe.MissingScopes[0] != "task:attachment:write" {
		t.Errorf("MissingScopes = %#v, want task:attachment:write", pe.MissingScopes)
	}
	if !strings.Contains(pe.Hint, pe.ConsoleURL) {
		t.Errorf("hint = %q, want to include console URL %q", pe.Hint, pe.ConsoleURL)
	}
}

func TestCallTaskAPITyped_TaskHint(t *testing.T) {
	cfg := taskTestConfig(t)
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+x"}, cfg, f, core.AsUser)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/task/v2/tasks/t-1",
		Body: map[string]interface{}{
			"code": float64(ErrCodeTaskReminderExists),
			"msg":  "reminder exists",
		},
	})

	_, err := callTaskAPITyped(rt, http.MethodGet, "/open-apis/task/v2/tasks/t-1", nil, nil)
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %T, want typed errs.* error", err)
	}
	if p.Hint != taskAPIHints[ErrCodeTaskReminderExists] {
		t.Errorf("hint = %q, want %q", p.Hint, taskAPIHints[ErrCodeTaskReminderExists])
	}
}

// TestHandleTaskApiResult_MalformedResponse covers the two malformed-body arms:
// a response with no top-level code, and one whose code is non-numeric. Both
// must surface a typed internal invalid_response error (exit 5) rather than
// silently passing through as a success.
func TestHandleTaskApiResult_MalformedResponse(t *testing.T) {
	cases := []struct {
		name   string
		result map[string]interface{}
	}{
		{"missing code field", map[string]interface{}{"msg": "weird", "data": map[string]interface{}{}}},
		{"non-numeric code", map[string]interface{}{"code": "oops", "msg": "weird"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := HandleTaskApiResult(tc.result, nil, "do thing")
			if data != nil {
				t.Errorf("data = %v, want nil", data)
			}
			var ie *errs.InternalError
			if !errors.As(err, &ie) {
				t.Fatalf("err = %T, want *errs.InternalError", err)
			}
			if ie.Subtype != errs.SubtypeInvalidResponse {
				t.Errorf("subtype = %q, want %q", ie.Subtype, errs.SubtypeInvalidResponse)
			}
			if got := output.ExitCodeOf(err); got != output.ExitInternal {
				t.Errorf("exit code = %d, want %d", got, output.ExitInternal)
			}
		})
	}
}

// TestHandleTaskApiResult_Success returns the data map unchanged when code == 0.
func TestHandleTaskApiResult_Success(t *testing.T) {
	want := map[string]interface{}{"guid": "t-1"}
	data, err := HandleTaskApiResult(map[string]interface{}{
		"code": float64(0),
		"data": want,
	}, nil, "do thing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["guid"] != "t-1" {
		t.Errorf("data = %v, want guid=t-1", data)
	}
}

// TestHandleTaskApiResult_UnknownCode covers the fallback arm: an uncatalogued
// code becomes a generic CategoryAPI error with SubtypeUnknown and no layered
// hint.
func TestHandleTaskApiResult_UnknownCode(t *testing.T) {
	_, err := HandleTaskApiResult(apiResult(9999999, "weird"), nil, "do thing")
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %T, want typed errs.* error", err)
	}
	if p.Subtype != errs.SubtypeUnknown {
		t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypeUnknown)
	}
	if p.Code != 9999999 {
		t.Errorf("code = %d, want 9999999", p.Code)
	}
	if got := output.ExitCodeOf(err); got != output.ExitAPI {
		t.Errorf("exit code = %d, want %d", got, output.ExitAPI)
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Errorf("error type = %T, want *errs.APIError", err)
	}
}
