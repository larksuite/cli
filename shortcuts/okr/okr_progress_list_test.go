// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func progressListTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-progress-list",
		AppSecret: "secret-okr-progress-list",
		Brand:     core.BrandFeishu,
	}
}

func runProgressListShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRListProgress.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestProgressListValidate_MissingTargetID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for missing --target-id")
	}
}

func TestProgressListValidate_MissingTargetType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "123",
	})
	if err == nil {
		t.Fatal("expected error for missing --target-type")
	}
}

func TestProgressListValidate_InvalidTargetID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "abc",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for non-numeric --target-id")
	}
	if !strings.Contains(err.Error(), "--target-id must be a positive int64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressListValidate_InvalidTargetType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "123",
		"--target-type", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid --target-type")
	}
	if !strings.Contains(err.Error(), "--target-type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressListValidate_InvalidUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "123",
		"--target-type", "objective",
		"--user-id-type", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid --user-id-type")
	}
}

func TestProgressListValidate_InvalidDepartmentIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "123",
		"--target-type", "objective",
		"--department-id-type", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid --department-id-type")
	}
}

func TestProgressListValidate_InvalidPageSize(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "123",
		"--target-type", "objective",
		"--page-size", "0",
	})
	if err == nil {
		t.Fatal("expected error for invalid --page-size")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected validation invalid_argument problem, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok || validationErr.Param != "--page-size" {
		t.Fatalf("expected param --page-size, got: %v", err)
	}
}

// --- DryRun tests ---

func TestProgressListDryRun_Objective(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "123456789",
		"--target-type", "objective",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v2/objectives/123456789/progresses") {
		t.Fatalf("dry-run output should contain objective API path, got: %s", output)
	}
	if !strings.Contains(output, "GET") {
		t.Fatalf("dry-run output should contain GET method, got: %s", output)
	}
	if !strings.Contains(output, "\"page_size\": 100") {
		t.Fatalf("dry-run output should contain default page_size=100, got: %s", output)
	}
}

func TestProgressListDryRun_KeyResult(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "987654321",
		"--target-type", "key_result",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v2/key_results/987654321/progresses") {
		t.Fatalf("dry-run output should contain key_result API path, got: %s", output)
	}
}

func TestProgressListDryRun_WithPagination(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressListTestConfig(t))
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "123456789",
		"--target-type", "objective",
		"--page-size", "25",
		"--page-token", "next-page",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "\"page_size\": 25") {
		t.Fatalf("dry-run output should contain page_size=25, got: %s", output)
	}
	if !strings.Contains(output, "\"page_token\": \"next-page\"") {
		t.Fatalf("dry-run output should contain page_token, got: %s", output)
	}
}

// --- Execute tests ---

func TestProgressListExecute_Success_Objective(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressListTestConfig(t))
	var gotQuery url.Values
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/123456789/progresses",
		OnMatch: func(req *http.Request) {
			gotQuery = req.URL.Query()
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":          "111",
						"create_time": "1735776000000",
						"update_time": "1735776100000",
						"owner":       map[string]interface{}{"owner_type": "user", "user_id": "ou_test"},
						"entity_type": 2,
						"entity_id":   "123456789",
						"content":     map[string]interface{}{"blocks": []interface{}{}},
						"progress_rate": map[string]interface{}{
							"progress_percent": 50.0,
							"progress_status":  0,
						},
					},
				},
				"has_more":   true,
				"page_token": "next_page",
			},
		},
	})
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "123456789",
		"--target-type", "objective",
		"--page-size", "50",
		"--page-token", "start_page",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := gotQuery.Get("page_size"); got != "50" {
		t.Fatalf("query page_size = %q, want 50", got)
	}
	if got := gotQuery.Get("page_token"); got != "start_page" {
		t.Fatalf("query page_token = %q, want start_page", got)
	}
	data := decodeEnvelope(t, stdout)
	records, _ := data["progress_list"].([]interface{})
	if len(records) != 1 {
		t.Fatalf("expected 1 progress, got %d", len(records))
	}
	if _, ok := data["total"]; ok {
		t.Fatal("total should not be present in response")
	}
	if hasMore, _ := data["has_more"].(bool); !hasMore {
		t.Fatalf("has_more = %v, want true", hasMore)
	}
	if pageToken, _ := data["page_token"].(string); pageToken != "next_page" {
		t.Fatalf("page_token = %q, want next_page", pageToken)
	}
}

func TestProgressListExecute_Success_KeyResult(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressListTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/key_results/987654321/progresses",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":          "222",
						"create_time": "1735776000000",
						"update_time": "1735776100000",
						"owner":       map[string]interface{}{"owner_type": "user", "user_id": "ou_test"},
						"entity_type": 3,
						"entity_id":   "987654321",
						"progress_rate": map[string]interface{}{
							"progress_percent": 100.0,
							"progress_status":  2,
						},
					},
				},
				"has_more": false,
			},
		},
	})
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "987654321",
		"--target-type", "key_result",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	records, _ := data["progress_list"].([]interface{})
	if len(records) != 1 {
		t.Fatalf("expected 1 progress, got %d", len(records))
	}
	record := records[0].(map[string]interface{})
	pr := record["progress_rate"].(map[string]interface{})
	if pr["status"] != "done" {
		t.Fatalf("progress status = %v, want done", pr["status"])
	}
}

func TestProgressListExecute_EmptyList(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressListTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/123456789/progresses",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{},
				"has_more": false,
			},
		},
	})
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "123456789",
		"--target-type", "objective",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	records, _ := data["progress_list"].([]interface{})
	if len(records) != 0 {
		t.Fatalf("expected 0 progress, got %d", len(records))
	}
}

func TestProgressListExecute_APIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressListTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/999/progresses",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "internal error",
		},
	})
	err := runProgressListShortcut(t, f, stdout, []string{
		"+progress-list",
		"--target-id", "999",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}
