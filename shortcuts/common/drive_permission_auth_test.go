// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"net/http"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestCheckDriveFileExportPermissionReturnsAuthResultAndSendsExportQuery(t *testing.T) {
	runtime, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/permissions/file_allowed/members/auth",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"auth_result": true},
		},
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("type"); got != drivePermissionResourceTypeFile {
				t.Errorf("type query = %q, want %q", got, drivePermissionResourceTypeFile)
			}
			if got := req.URL.Query().Get("action"); got != drivePermissionActionExport {
				t.Errorf("action query = %q, want %q", got, drivePermissionActionExport)
			}
		},
	})

	allowed, err := CheckDriveFileExportPermission(runtime, "file_allowed")
	if err != nil {
		t.Fatalf("CheckDriveFileExportPermission() error = %v", err)
	}
	if !allowed {
		t.Fatal("CheckDriveFileExportPermission() allowed = false, want true")
	}
}

func TestCheckDriveFileExportPermissionReturnsFalseWithoutError(t *testing.T) {
	runtime, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/permissions/file_denied/members/auth",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"auth_result": false},
		},
	})

	allowed, err := CheckDriveFileExportPermission(runtime, "file_denied")
	if err != nil {
		t.Fatalf("CheckDriveFileExportPermission() error = %v", err)
	}
	if allowed {
		t.Fatal("CheckDriveFileExportPermission() allowed = true, want false")
	}
}

func TestCheckDriveFileExportPermissionRejectsMalformedAuthResult(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string]interface{}
	}{
		{name: "missing", data: map[string]interface{}{}},
		{name: "string", data: map[string]interface{}{"auth_result": "true"}},
		{name: "number", data: map[string]interface{}{"auth_result": float64(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime, reg := newCallAPITypedRuntime(t)
			reg.Register(&httpmock.Stub{
				Method: http.MethodGet,
				URL:    "/open-apis/drive/v1/permissions/file_malformed/members/auth",
				Body: map[string]interface{}{
					"code": 0,
					"data": tc.data,
				},
			})

			_, err := CheckDriveFileExportPermission(runtime, "file_malformed")
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed error, got %T: %v", err, err)
			}
			if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("problem = category %q subtype %q, want internal/invalid_response", problem.Category, problem.Subtype)
			}
		})
	}
}

func TestCheckDriveFileExportPermissionPassesThroughTypedAPIError(t *testing.T) {
	runtime, reg := newCallAPITypedRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/drive/v1/permissions/file_limited/members/auth",
		Body: map[string]interface{}{
			"code":   99991400,
			"msg":    "rate limited",
			"log_id": "log-auth-rate-limit",
		},
	})

	_, err := CheckDriveFileExportPermission(runtime, "file_limited")
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeRateLimit || problem.Code != 99991400 {
		t.Fatalf("problem = %+v, want api/rate_limit/99991400", problem)
	}
	if problem.LogID != "log-auth-rate-limit" || !problem.Retryable {
		t.Fatalf("problem = %+v, want preserved log_id and retryable", problem)
	}
}
