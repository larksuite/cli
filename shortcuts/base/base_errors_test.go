// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
)

func TestErrorDetailHelpers(t *testing.T) {
	detail := map[string]interface{}{
		"message": "boom",
		"hint":    "retry later",
		"extra":   "keep me",
	}
	wantDetail := map[string]interface{}{
		"message": "boom",
		"hint":    "retry later",
		"extra":   "keep me",
	}
	if got := extractErrorHint(map[string]interface{}{"data": map[string]interface{}{"error": detail}}); got != "retry later" {
		t.Fatalf("hint=%q", got)
	}
	if got := extractDataErrorMessage(map[string]interface{}{"data": map[string]interface{}{"error": detail}}); got != "boom" {
		t.Fatalf("message=%q", got)
	}
	if got := extractDataErrorMessage(map[string]interface{}{"data": map[string]interface{}{}}); got != "" {
		t.Fatalf("message=%q", got)
	}
	if !reflect.DeepEqual(detail, wantDetail) {
		t.Fatalf("detail mutated: got %#v, want %#v", detail, wantDetail)
	}
}

func TestHandleBaseAPIResultErrorPaths(t *testing.T) {
	if _, err := handleBaseAPIResultAny(nil, assertErr{}, "list fields"); err == nil || !strings.Contains(err.Error(), "list fields") {
		t.Fatalf("err=%v", err)
	}
	result := map[string]interface{}{
		"code": 190001,
		"msg":  "bad request",
		"data": map[string]interface{}{
			"error": map[string]interface{}{"message": "invalid filter", "hint": "check field name"},
		},
	}
	if _, err := handleBaseAPIResultAny(result, nil, "set filter"); err == nil || !strings.Contains(err.Error(), "invalid filter") {
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
	if _, err := handleBaseAPIResult(result, nil, "set filter"); err == nil {
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

	_, err := handleBaseAPIResultAny(result, nil, "API call failed")
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

func TestHandleBaseAPIResultHintPrecedence(t *testing.T) {
	t.Run("field violations win over Base hint", func(t *testing.T) {
		errorBlock := map[string]interface{}{
			"hint": "generic Base hint",
			"field_violations": []interface{}{
				map[string]interface{}{
					"field":       "fields.Amount",
					"description": "must be a number",
				},
			},
		}
		result := map[string]interface{}{
			"code":  190001,
			"msg":   "invalid field",
			"error": errorBlock,
		}

		_, err := handleBaseAPIResultAny(result, nil, "update record")
		var apiErr *errs.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected typed error, got %T %v", err, err)
		}
		if got, want := apiErr.Hint, "fields.Amount: must be a number"; got != want {
			t.Fatalf("hint = %q, want field violation hint %q", got, want)
		}
		if !apiErr.HintIsFromServer() {
			t.Fatal("field violation hint must retain server provenance")
		}
		if !reflect.DeepEqual(errorBlock, map[string]interface{}{
			"hint": "generic Base hint",
			"field_violations": []interface{}{
				map[string]interface{}{"field": "fields.Amount", "description": "must be a number"},
			},
		}) {
			t.Fatalf("top-level error mutated: %#v", errorBlock)
		}
	})

	t.Run("data.error field violations win without mutating caller", func(t *testing.T) {
		dataError := map[string]interface{}{
			"message": "data error message",
			"hint":    "generic data Base hint",
			"logid":   "data-log-id",
			"field_violations": []interface{}{
				map[string]interface{}{
					"field":       "fields.Amount",
					"description": "must be a number",
				},
			},
			"extra_context": "keep me",
		}
		wantDataError := map[string]interface{}{
			"message": "data error message",
			"hint":    "generic data Base hint",
			"logid":   "data-log-id",
			"field_violations": []interface{}{
				map[string]interface{}{
					"field":       "fields.Amount",
					"description": "must be a number",
				},
			},
			"extra_context": "keep me",
		}
		topError := map[string]interface{}{
			"troubleshooter": "https://example.test/base-troubleshooter",
		}
		result := map[string]interface{}{
			"code":  190001,
			"msg":   "outer message",
			"error": topError,
			"data":  map[string]interface{}{"error": dataError},
		}

		_, err := handleBaseAPIResultAny(result, nil, "update record")
		var apiErr *errs.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *errs.APIError, got %T %v", err, err)
		}
		if apiErr.Message != "data error message" || apiErr.LogID != "data-log-id" {
			t.Fatalf("problem = %+v, want projected data.error message/logid", apiErr.Problem)
		}
		if apiErr.Troubleshooter != "https://example.test/base-troubleshooter" {
			t.Fatalf("Troubleshooter = %q", apiErr.Troubleshooter)
		}
		if got, want := apiErr.Hint, "fields.Amount: must be a number"; got != want {
			t.Fatalf("Hint = %q, want field violation hint %q", got, want)
		}
		if len(apiErr.FieldViolations) != 1 || !apiErr.HintIsFromServer() {
			t.Fatalf("APIError = %+v, want one server-provenance field violation", apiErr)
		}
		if result["msg"] != "outer message" {
			t.Fatalf("caller msg mutated to %#v", result["msg"])
		}
		if _, exists := result["log_id"]; exists {
			t.Fatalf("caller result gained top-level log_id: %#v", result)
		}
		if !reflect.DeepEqual(dataError, wantDataError) {
			t.Fatalf("data.error mutated: got %#v, want %#v", dataError, wantDataError)
		}
		if !reflect.DeepEqual(topError, map[string]interface{}{
			"troubleshooter": "https://example.test/base-troubleshooter",
		}) {
			t.Fatalf("top-level error mutated: %#v", topError)
		}
	})

	t.Run("Base hint remains the fallback without violations", func(t *testing.T) {
		errorBlock := map[string]interface{}{"hint": "generic Base hint"}
		result := map[string]interface{}{
			"code":  190001,
			"msg":   "invalid field",
			"error": errorBlock,
		}

		_, err := handleBaseAPIResultAny(result, nil, "update record")
		var apiErr *errs.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected typed error, got %T %v", err, err)
		}
		if got, want := apiErr.Hint, "generic Base hint"; got != want {
			t.Fatalf("hint = %q, want Base hint %q", got, want)
		}
		if !apiErr.HintIsFromServer() {
			t.Fatal("Base hint override must carry server provenance")
		}
		if !reflect.DeepEqual(errorBlock, map[string]interface{}{"hint": "generic Base hint"}) {
			t.Fatalf("top-level error mutated: %#v", errorBlock)
		}
	})
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

	_, err := handleBaseAPIResultAny(result, nil, "API call failed")
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

	_, err := handleBaseAPIResultAny(result, nil, "API call failed")
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
		_, err := handleBaseAPIResultAny(result, nil, "list tables")
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

	t.Run("body hint adopts server provenance", func(t *testing.T) {
		outer := errs.NewAPIError(errs.SubtypeConflict, "outer failure").
			WithFallbackHint("outer fallback").
			WithCode(190001)
		body := []byte("{\"code\":190001,\"msg\":\"boom\",\"data\":{\"error\":{\"hint\":\"body server hint\"}}}")
		err := enrichBaseAPIErrorFromBody(outer, body, errclass.ClassifyContext{})
		var apiErr *errs.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *errs.APIError, got %T %v", err, err)
		}
		if apiErr != outer {
			t.Fatal("enrichment must update and return the original typed error")
		}
		if apiErr.Hint != "body server hint" || !apiErr.HintIsFromServer() {
			t.Fatalf("APIError = %+v, want body hint with server provenance", apiErr)
		}
	})

	t.Run("body field violations replace outer diagnostics without aliasing", func(t *testing.T) {
		outer := errs.NewAPIError(errs.SubtypeConflict, "outer failure").
			WithFallbackHint("outer fallback").
			WithCode(190001)
		outer.FieldViolations = []errs.APIFieldViolation{{
			Field: "old.field", Description: "old reason",
		}}
		body := []byte("{\"code\":190001,\"msg\":\"boom\",\"data\":{\"error\":{\"hint\":\"body generic hint\",\"field_violations\":[{\"field\":\"fields.Amount\",\"value\":\"abc\",\"description\":\"must be a number\"}]}}}")
		err := enrichBaseAPIErrorFromBody(outer, body, errclass.ClassifyContext{})
		var apiErr *errs.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *errs.APIError, got %T %v", err, err)
		}
		want := []errs.APIFieldViolation{{
			Field: "fields.Amount", Value: "abc", Description: "must be a number",
		}}
		if apiErr.Hint != "fields.Amount: must be a number" || !apiErr.HintIsFromServer() {
			t.Fatalf("APIError = %+v, want body field hint with server provenance", apiErr)
		}
		if !reflect.DeepEqual(apiErr.FieldViolations, want) {
			t.Fatalf("FieldViolations = %#v, want %#v", apiErr.FieldViolations, want)
		}

		source := baseAPIErrorFromResult(map[string]interface{}{
			"code": 190001,
			"msg":  "boom",
			"data": map[string]interface{}{
				"error": map[string]interface{}{
					"field_violations": []interface{}{
						map[string]interface{}{
							"field": "fields.Amount", "value": "abc", "description": "must be a number",
						},
					},
				},
			},
		}, errclass.ClassifyContext{})
		var sourceAPI *errs.APIError
		if !errors.As(source, &sourceAPI) {
			t.Fatalf("source = %T, want *errs.APIError", source)
		}
		adopted := errs.NewAPIError(errs.SubtypeConflict, "outer").AdoptServerDiagnostics(sourceAPI)
		sourceAPI.FieldViolations[0].Field = "mutated.source"
		if got := adopted.FieldViolations[0].Field; got != "fields.Amount" {
			t.Fatalf("adopted violations alias source: got %q", got)
		}
	})

	t.Run("body without field violations preserves outer diagnostics", func(t *testing.T) {
		outer := errs.NewAPIError(errs.SubtypeConflict, "outer failure").
			WithFallbackHint("outer fallback").
			WithCode(190001)
		outer.FieldViolations = []errs.APIFieldViolation{{
			Field: "outer.field", Description: "outer reason",
		}}
		body := []byte("{\"code\":190001,\"msg\":\"boom\",\"data\":{\"error\":{\"hint\":\"body server hint\"}}}")
		err := enrichBaseAPIErrorFromBody(outer, body, errclass.ClassifyContext{})
		var apiErr *errs.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *errs.APIError, got %T %v", err, err)
		}
		if apiErr.Hint != "body server hint" || !apiErr.HintIsFromServer() {
			t.Fatalf("APIError = %+v, want body hint with server provenance", apiErr)
		}
		want := []errs.APIFieldViolation{{Field: "outer.field", Description: "outer reason"}}
		if !reflect.DeepEqual(apiErr.FieldViolations, want) {
			t.Fatalf("FieldViolations = %#v, want preserved %#v", apiErr.FieldViolations, want)
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
