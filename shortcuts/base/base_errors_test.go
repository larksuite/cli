// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
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

func TestBaseUpstreamErrorPreservesShapeAndCause(t *testing.T) {
	for _, placement := range []string{"nested", "top-level"} {
		t.Run(placement, func(t *testing.T) {
			detail := json.RawMessage(`{"type":"not_found","message":"missing","hint":"list fields","retryable":false,"value":9007199254740993,"table":null,"extra":{"future":true}}`)
			body := map[string]interface{}{"code": 800030201, "msg": "missing"}
			if placement == "nested" {
				body["data"] = map[string]interface{}{"error": detail}
			} else {
				body["error"] = detail
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			cause := assertErr{}
			original := errs.NewAPIError(errs.SubtypeNotFound, "original").WithCode(800030201).WithCause(cause)
			enriched := enrichBaseAPIErrorFromBody(original, raw, errclass.ClassifyContext{})
			if enriched != original || !errors.Is(enriched, cause) {
				t.Fatal("lost error identity/cause")
			}
			problem, _ := errs.ProblemOf(enriched)
			// Compare decoded values using json.Number so large integers remain exact.
			wantMap := map[string]interface{}{}
			gotMap := map[string]interface{}{}
			for _, item := range []struct {
				raw    []byte
				target *map[string]interface{}
			}{{detail, &wantMap}, {problem.UpstreamError, &gotMap}} {
				decoder := json.NewDecoder(bytes.NewReader(item.raw))
				decoder.UseNumber()
				if err := decoder.Decode(item.target); err != nil {
					t.Fatal(err)
				}
			}
			want, _ := json.Marshal(wantMap)
			got, _ := json.Marshal(gotMap)
			if !bytes.Equal(want, got) {
				t.Fatalf("detail=%s, want=%s", got, want)
			}
		})
	}
}
