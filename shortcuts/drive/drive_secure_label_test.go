// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDriveSecureLabelScopes(t *testing.T) {
	t.Parallel()

	if len(DriveSecureLabelList.Scopes) != 1 || DriveSecureLabelList.Scopes[0] != "docs:secure_label:readonly" {
		t.Fatalf("list scopes = %v, want docs:secure_label:readonly", DriveSecureLabelList.Scopes)
	}
	if len(DriveSecureLabelUpdate.Scopes) != 1 || DriveSecureLabelUpdate.Scopes[0] != "docs:secure_label:write_only" {
		t.Fatalf("update scopes = %v, want docs:secure_label:write_only", DriveSecureLabelUpdate.Scopes)
	}
}

func TestDriveSecureLabelList_DryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveSecureLabelList, []string{
		"+secure-label-list",
		"--page-size", "5",
		"--page-token", "page_1",
		"--lang", "zh",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"/open-apis/drive/v2/my_secure_labels",
		`"GET"`,
		`"page_size": 5`,
		`"page_token": "page_1"`,
		`"lang": "zh"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestDriveSecureLabelList_ValidatePageSize(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveSecureLabelList, []string{
		"+secure-label-list",
		"--page-size", "11",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "page-size") {
		t.Fatalf("expected page-size validation error, got: %v", err)
	}
}

func TestDriveSecureLabelList_ExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v2/my_secure_labels?page_size=10",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "7217780879644737540", "name": "L1"},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveSecureLabelList, []string{
		"+secure-label-list",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"L1"`) {
		t.Fatalf("stdout missing label:\n%s", stdout.String())
	}
}

func TestDriveSecureLabelList_RateLimitPreservesUpstreamHint(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v2/my_secure_labels?page_size=10",
		Status: 429,
		Body: map[string]interface{}{
			"code": 99991400,
			"msg":  "rate limit exceeded",
			"error": map[string]interface{}{
				"details": []interface{}{
					map[string]interface{}{"value": "server says slow down"},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveSecureLabelList, []string{
		"+secure-label-list",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.Subtype != errs.SubtypeRateLimit || apiErr.Code != 99991400 || !apiErr.Retryable {
		t.Fatalf("problem = %+v, want code=99991400 subtype=rate_limit retryable=true", apiErr.Problem)
	}
	for _, want := range []string{"server says slow down", "secure label listing is rate limited"} {
		if !strings.Contains(apiErr.Hint, want) {
			t.Fatalf("hint missing %q: %q", want, apiErr.Hint)
		}
	}
	if strings.Contains(apiErr.Hint, "updates are rate limited") {
		t.Fatalf("list hint should not use update-specific wording: %q", apiErr.Hint)
	}
}

func TestDriveSecureLabelUpdate_DryRunInfersTypeFromURL(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveSecureLabelUpdate, []string{
		"+secure-label-update",
		"--token", "https://example.feishu.cn/docx/doxTok123?from=share",
		"--label-id", " 7217780879644737539 ",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"/open-apis/drive/v2/files/doxTok123/secure_label",
		`"PATCH"`,
		`"docx"`,
		`"id": "7217780879644737539"`,
		`"file_token": "doxTok123"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestDriveSecureLabelUpdate_ExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v2/files/doxTok123/secure_label?type=docx",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveSecureLabelUpdate, []string{
		"+secure-label-update",
		"--token", "doxTok123",
		"--type", "docx",
		"--label-id", " 7217780879644737539 ",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if body["id"] != "7217780879644737539" {
		t.Fatalf("id = %v, want label id", body["id"])
	}
}

func TestDriveSecureLabelUpdate_RejectsDisplayNameAsLabelID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveSecureLabelUpdate, []string{
		"+secure-label-update",
		"--token", "doxTok123",
		"--type", "docx",
		"--label-id", "Public(D)",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected label id validation error")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--label-id" {
		t.Fatalf("Param = %q, want --label-id", validationErr.Param)
	}
	if !strings.Contains(validationErr.Hint, "+secure-label-list") {
		t.Fatalf("hint missing list guidance: %q", validationErr.Hint)
	}
}

func TestDriveSecureLabelUpdate_DowngradeApprovalReturnsFailedPrecondition(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v2/files/doxTok123/secure_label",
		Status: 403,
		Body: map[string]interface{}{
			"code": 1063013, "msg": "Security label downgrade requires approval",
		},
	})

	targetURL := "https://example.feishu.cn/docx/doxTok123"
	err := mountAndRunDrive(t, DriveSecureLabelUpdate, []string{
		"+secure-label-update",
		"--token", targetURL,
		"--label-id", "7217780879644737539",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected 1063013 error")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Subtype != errs.SubtypeFailedPrecondition || validationErr.Code != 1063013 {
		t.Fatalf("problem = %+v, want code=1063013 subtype=failed_precondition", validationErr.Problem)
	}
	if !strings.Contains(validationErr.Hint, "approval") {
		t.Fatalf("hint missing approval guidance: %q", validationErr.Hint)
	}
}

func TestDriveSecureLabelUpdate_InvalidJSONTypeGetsLabelHint(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v2/files/doxTok123/secure_label",
		Status: 400,
		Body: map[string]interface{}{
			"code": 9499, "msg": "Invalid parameter type in json: id",
		},
	})

	err := mountAndRunDrive(t, DriveSecureLabelUpdate, []string{
		"+secure-label-update",
		"--token", "https://example.feishu.cn/docx/doxTok123",
		"--label-id", "7217780879644737539",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected 9499 error")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.Subtype != errs.SubtypeInvalidParameters || apiErr.Code != 9499 {
		t.Fatalf("problem = %+v, want code=9499 subtype=invalid_parameters", apiErr.Problem)
	}
	if !strings.Contains(apiErr.Hint, "+secure-label-list") {
		t.Fatalf("hint missing secure label list guidance: %q", apiErr.Hint)
	}
}

func TestDriveSecureLabelUpdate_RateLimitIsRetryableWithBackoffHint(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v2/files/doxTok123/secure_label",
		Status: 429,
		Body: map[string]interface{}{
			"code": 99991400, "msg": "rate limit exceeded",
		},
	})

	err := mountAndRunDrive(t, DriveSecureLabelUpdate, []string{
		"+secure-label-update",
		"--token", "https://example.feishu.cn/docx/doxTok123",
		"--label-id", "7217780879644737539",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.Subtype != errs.SubtypeRateLimit || apiErr.Code != 99991400 || !apiErr.Retryable {
		t.Fatalf("problem = %+v, want code=99991400 subtype=rate_limit retryable=true", apiErr.Problem)
	}
	if !strings.Contains(apiErr.Hint, "backoff") {
		t.Fatalf("hint missing backoff guidance: %q", apiErr.Hint)
	}
}
