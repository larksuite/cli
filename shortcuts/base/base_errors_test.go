// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestErrorDetailHelpers(t *testing.T) {
	detail := map[string]interface{}{"message": "boom", "hint": "retry later"}
	if got := extractErrorHint(map[string]interface{}{"data": map[string]interface{}{"error": detail}}); got != "retry later" {
		t.Fatalf("hint=%q", got)
	}
	if got := extractDataErrorMessage(map[string]interface{}{"data": map[string]interface{}{"error": detail}}); got != "boom" {
		t.Fatalf("message=%q", got)
	}
	if got := extractDataErrorMessage(map[string]interface{}{"data": map[string]interface{}{}}); got != "" {
		t.Fatalf("message=%q", got)
	}
}

func TestHandleBaseAPIResultErrorPaths(t *testing.T) {
	if _, err := handleBaseAPIResultAny(nil, nil, assertErr{}, "list fields"); err == nil || !strings.Contains(err.Error(), "list fields") {
		t.Fatalf("err=%v", err)
	}
	result := map[string]interface{}{
		"code": 190001,
		"msg":  "bad request",
		"data": map[string]interface{}{
			"error": map[string]interface{}{"message": "invalid filter", "hint": "check field name"},
		},
	}
	if _, err := handleBaseAPIResultAny(nil, result, nil, "set filter"); err == nil || !strings.Contains(err.Error(), "invalid filter") {
		t.Fatalf("err=%v", err)
	} else {
		p, ok := errs.ProblemOf(err)
		if !ok || p.Code != 190001 {
			t.Fatalf("expected typed code 190001, got %T %v", err, err)
		}
		if p.Hint != "check field name" {
			t.Fatalf("hint=%q", p.Hint)
		}
	}
	if _, err := handleBaseAPIResult(nil, result, nil, "set filter"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleBaseAPIResultPromotesBaseErrorFields(t *testing.T) {
	result := map[string]interface{}{
		"code": 800010407,
		"msg":  "cell value invalid",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"docs_url":       nil,
				"hint":           "Provide a number value.",
				"level":          "error",
				"logid":          "20260508160000000000000000000000",
				"message":        "The cell value does not match the expected input shape.",
				"path":           "Amount",
				"retry_after_ms": nil,
				"retryable":      false,
				"extra_context":  "future detail field",
				"table":          map[string]interface{}{"id": "tbl_1", "name": "Orders"},
				"type":           "invalid_request",
				"upstream_code":  nil,
				"value":          "abc",
			},
		},
	}

	_, err := handleBaseAPIResultAny(nil, result, nil, "API call failed")
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if p.Code != 800010407 {
		t.Fatalf("code=%d", p.Code)
	}
	if p.Message != "The cell value does not match the expected input shape." {
		t.Fatalf("message=%q", p.Message)
	}
	if p.Hint != "Provide a number value." {
		t.Fatalf("hint=%q", p.Hint)
	}
	if p.LogID != "20260508160000000000000000000000" {
		t.Fatalf("logID=%q", p.LogID)
	}
}

func TestHandleBaseAPIResultClassifiesKnownPermissionCode(t *testing.T) {
	result := map[string]interface{}{
		"code": 99991676,
		"msg":  "permission denied",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"hint":    "Grant base:record:read to the app.",
				"message": "Missing required scope base:record:read.",
			},
		},
	}

	_, err := handleBaseAPIResultAny(nil, result, nil, "API call failed")
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if p.Code != 99991676 {
		t.Fatalf("code=%d", p.Code)
	}
	if p.Category != errs.CategoryAuthorization || p.Subtype != errs.SubtypeTokenScopeInsufficient {
		t.Fatalf("category/subtype=%s/%s", p.Category, p.Subtype)
	}
}

func TestAttachBaseResponseLogIDFromHeader(t *testing.T) {
	result := map[string]interface{}{
		"code": 91402,
		"msg":  "NOTEXIST",
		"data": map[string]interface{}{},
	}
	attachBaseErrorLogID(result, "20260508170000000000000000000000")

	_, err := handleBaseAPIResultAny(nil, result, nil, "API call failed")
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if p.LogID != "20260508170000000000000000000000" {
		t.Fatalf("logID=%q", p.LogID)
	}
}

func TestHandleBaseAPIResultRejectsNonNumericCode(t *testing.T) {
	for _, code := range []interface{}{"oops", map[string]interface{}{}, nil} {
		result := map[string]interface{}{"code": code, "msg": "weird envelope"}
		_, err := handleBaseAPIResultAny(nil, result, nil, "list tables")
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("code=%#v: expected typed error, got %T %v", code, err, err)
		}
		if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
			t.Fatalf("code=%#v: category/subtype=%s/%s", code, p.Category, p.Subtype)
		}
		if !strings.Contains(p.Message, "list tables") {
			t.Fatalf("code=%#v: message=%q", code, p.Message)
		}
	}
}

func TestEnrichBaseAPIErrorFromBodyLogIDMerge(t *testing.T) {
	t.Run("body without log_id keeps header-derived LogID", func(t *testing.T) {
		outer := errs.NewAPIError(errs.SubtypeUnknown, "outer failure").WithCode(190001).WithLogID("header-log-id")
		err := enrichBaseAPIErrorFromBody(outer, []byte(`{"code":190001,"msg":"boom"}`), errclass.ClassifyContext{})
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("expected typed error, got %T %v", err, err)
		}
		if p.Message != "boom" {
			t.Fatalf("message=%q", p.Message)
		}
		if p.LogID != "header-log-id" {
			t.Fatalf("logID=%q, want header-log-id", p.LogID)
		}
	})

	t.Run("body log_id overrides header-derived LogID", func(t *testing.T) {
		outer := errs.NewAPIError(errs.SubtypeUnknown, "outer failure").WithCode(190001).WithLogID("header-log-id")
		body := `{"code":190001,"msg":"boom","data":{"error":{"logid":"body-log-id"}}}`
		err := enrichBaseAPIErrorFromBody(outer, []byte(body), errclass.ClassifyContext{})
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("expected typed error, got %T %v", err, err)
		}
		if p.LogID != "body-log-id" {
			t.Fatalf("logID=%q, want body-log-id", p.LogID)
		}
	})
}

func TestBaseMissingFileIOErrorIsInternal(t *testing.T) {
	p, ok := errs.ProblemOf(baseMissingFileIOError("file operations require a FileIO provider"))
	if !ok {
		t.Fatal("expected typed error")
	}
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeFileIO {
		t.Fatalf("category/subtype=%s/%s", p.Category, p.Subtype)
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "network timeout" }

func assertProblemCode(t *testing.T, err error, code int, messageParts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %d", code)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T %v", err, err)
	}
	if p.Code != code {
		t.Fatalf("code=%d, want %d; err=%v", p.Code, code, err)
	}
	for _, part := range messageParts {
		if !strings.Contains(p.Message, part) {
			t.Fatalf("message=%q missing %q", p.Message, part)
		}
	}
}

// Every Base v3 shortcut is available to an Agent Employee, so an authorization
// failure there has to route recovery to the Agent platform: an Agent Employee
// has nobody to complete a device login, and `auth login` refuses its profile
// outright. The human recovery must stay untouched for everyone else.
func TestHandleBaseAPIResultRoutesRecoveryByCredentialKind(t *testing.T) {
	envelope := func() map[string]interface{} {
		return map[string]interface{}{"code": 99991679, "msg": "user access token invalid"}
	}
	runtimeFor := func(kind core.CredentialKind) *common.RuntimeContext {
		return &common.RuntimeContext{Config: &core.CliConfig{
			AppID:          "cli_base_agent",
			Brand:          core.BrandFeishu,
			CredentialKind: kind,
		}}
	}

	_, err := handleBaseAPIResultAny(runtimeFor(core.CredentialAAT), envelope(), nil, "API call failed")
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if strings.Contains(p.Hint, "auth login") {
		t.Fatalf("Agent Employee was sent to the human device login: %q", p.Hint)
	}
	if !strings.Contains(p.Hint, "Agent Employee") {
		t.Fatalf("hint does not name the Agent platform: %q", p.Hint)
	}

	_, err = handleBaseAPIResultAny(runtimeFor(""), envelope(), nil, "API call failed")
	p, ok = errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if !strings.Contains(p.Hint, "auth login") {
		t.Fatalf("a human profile must keep the device-login recovery: %q", p.Hint)
	}
}
