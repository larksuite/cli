// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func progressGetTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-progress-get",
		AppSecret: "secret-okr-progress-get",
		Brand:     core.BrandFeishu,
	}
}

func runProgressGetShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRGetProgressRecord.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestProgressGetValidate_MissingProgressID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressGetTestConfig(t))
	err := runProgressGetShortcut(t, f, stdout, []string{"+progress-get"})
	if err == nil {
		t.Fatal("expected error for missing --progress-id")
	}
	if !strings.Contains(err.Error(), "progress-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressGetValidate_InvalidProgressID_NonNumeric(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressGetTestConfig(t))
	err := runProgressGetShortcut(t, f, stdout, []string{"+progress-get", "--progress-id", "abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric --progress-id")
	}
	if !strings.Contains(err.Error(), "--progress-id must be a positive int64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressGetValidate_InvalidProgressID_Zero(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressGetTestConfig(t))
	err := runProgressGetShortcut(t, f, stdout, []string{"+progress-get", "--progress-id", "0"})
	if err == nil {
		t.Fatal("expected error for zero --progress-id")
	}
}

func TestProgressGetValidate_InvalidUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressGetTestConfig(t))
	err := runProgressGetShortcut(t, f, stdout, []string{"+progress-get", "--progress-id", "123", "--user-id-type", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid --user-id-type")
	}
	if !strings.Contains(err.Error(), "--user-id-type must be one of") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressGetValidate_Valid(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressGetTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v1/progress_records/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"progress_id": "123",
				"modify_time": "1735776000000",
			},
		},
	})
	err := runProgressGetShortcut(t, f, stdout, []string{"+progress-get", "--progress-id", "123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- DryRun tests ---

func TestProgressGetDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressGetTestConfig(t))
	err := runProgressGetShortcut(t, f, stdout, []string{
		"+progress-get",
		"--progress-id", "456",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "456") {
		t.Fatalf("dry-run output should contain progress-id 456, got: %s", output)
	}
	if !strings.Contains(output, "/open-apis/okr/v1/progress_records/456") {
		t.Fatalf("dry-run output should contain API path, got: %s", output)
	}
}

// --- Execute tests ---

func TestProgressGetExecute_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressGetTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v1/progress_records/789",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"progress_id": "789",
				"modify_time": "1735776000000",
				"content": map[string]interface{}{
					"blocks": []interface{}{
						map[string]interface{}{
							"type": "paragraph",
							"paragraph": map[string]interface{}{
								"elements": []interface{}{
									map[string]interface{}{
										"type":    "textRun",
										"textRun": map[string]interface{}{"text": "ProgressV1 update"},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	err := runProgressGetShortcut(t, f, stdout, []string{"+progress-get", "--progress-id", "789"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	pr, _ := data["progress"].(map[string]interface{})
	if pr == nil {
		t.Fatal("expected progress in output")
	}
	if pr["progress_id"] != "789" {
		t.Fatalf("progress_id = %v, want 789", pr["progress_id"])
	}
}

func TestProgressGetExecute_APIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressGetTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v1/progress_records/999",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "internal error",
		},
	})
	err := runProgressGetShortcut(t, f, stdout, []string{"+progress-get", "--progress-id", "999"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}
