// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestErrorDetailHelpers(t *testing.T) {
	if value, ok := nonNilMapValue(nil, "error"); ok || value != nil {
		t.Fatalf("nil map should not return value")
	}
	if value, ok := nonNilMapValue(map[string]interface{}{"error": nil}, "error"); ok || value != nil {
		t.Fatalf("nil entry should not return value")
	}
	detail := map[string]interface{}{"message": "boom", "hint": "retry later"}
	if value, ok := nonNilMapValue(map[string]interface{}{"error": detail}, "error"); !ok || value == nil {
		t.Fatalf("expected non-nil detail")
	}
	if got := extractErrorDetail(map[string]interface{}{"error": detail}); got == nil {
		t.Fatalf("expected root detail")
	}
	if got := extractErrorDetail(map[string]interface{}{"data": map[string]interface{}{"error": detail}}); got == nil {
		t.Fatalf("expected nested detail")
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
		var exitErr *output.ExitError
		if !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Code != 190001 {
			t.Fatalf("expected structured code 190001, got %v", err)
		}
	}
	if _, err := handleBaseAPIResult(result, nil, "set filter"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleBaseAPIResultCleansBaseErrorDetail(t *testing.T) {
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
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected structured exit error, got %v", err)
	}

	errDetail := exitErr.Detail
	if errDetail.Code != 800010407 {
		t.Fatalf("code=%d", errDetail.Code)
	}
	if errDetail.Hint != "Provide a number value." {
		t.Fatalf("hint=%q", errDetail.Hint)
	}
	detail, _ := errDetail.Detail.(map[string]interface{})
	if detail == nil {
		t.Fatalf("expected cleaned detail, got %#v", errDetail.Detail)
	}
	if _, exists := detail["message"]; exists {
		t.Fatalf("detail should not repeat message: %#v", detail)
	}
	if _, exists := detail["hint"]; exists {
		t.Fatalf("detail should not repeat hint: %#v", detail)
	}
	if _, exists := detail["docs_url"]; exists {
		t.Fatalf("detail should omit nil docs_url: %#v", detail)
	}
	if detail["level"] != "error" {
		t.Fatalf("detail should preserve non-duplicate fields: %#v", detail)
	}
	if detail["extra_context"] != "future detail field" {
		t.Fatalf("detail should pass through unknown non-nil fields: %#v", detail)
	}
	if detail["path"] != "Amount" || detail["value"] != "abc" {
		t.Fatalf("cleaned detail mismatch: %#v", detail)
	}
	if detail["logid"] != "20260508160000000000000000000000" {
		t.Fatalf("logid=%q", detail["logid"])
	}
	if retryable, ok := detail["retryable"].(bool); !ok || retryable {
		t.Fatalf("retryable=%v", detail["retryable"])
	}
	table, _ := detail["table"].(map[string]interface{})
	if table["id"] != "tbl_1" || table["name"] != "Orders" {
		t.Fatalf("table=%#v", detail["table"])
	}
}

func TestHandleBaseAPIResultAlwaysRemovesMessageAndHintFromDetail(t *testing.T) {
	result := map[string]interface{}{
		"code": output.LarkErrTokenNoPermission,
		"msg":  "permission denied",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"hint":    "Grant base:record:read to the app.",
				"message": "Missing required scope base:record:read.",
			},
		},
	}

	_, err := handleBaseAPIResultAny(result, nil, "API call failed")
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected structured exit error, got %v", err)
	}
	if exitErr.Detail.Message != "Permission denied [99991676]" {
		t.Fatalf("message=%q", exitErr.Detail.Message)
	}
	if exitErr.Detail.Detail != nil {
		t.Fatalf("detail should be empty after removing message and hint: %#v", exitErr.Detail.Detail)
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
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected structured exit error, got %v", err)
	}
	detail, _ := exitErr.Detail.Detail.(map[string]interface{})
	if detail["logid"] != "20260508170000000000000000000000" {
		t.Fatalf("logid=%q", detail["logid"])
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "network timeout" }
