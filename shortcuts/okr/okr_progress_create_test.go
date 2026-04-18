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

func progressCreateTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-progress-create",
		AppSecret: "secret-okr-progress-create",
		Brand:     core.BrandFeishu,
	}
}

func runProgressCreateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRCreateProgressRecord.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

const validContentBlockJSON = `{"blocks":[{"block_element_type":"paragraph","paragraph":{"elements":[{"paragraph_element_type":"textRun","text_run":{"text":"test content"}}]}}]}`

// --- Validate tests ---

func TestProgressCreateValidate_MissingContent(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--target-id", "123",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for missing --content")
	}
}

func TestProgressCreateValidate_InvalidContentJSON(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", "not-json",
		"--target-id", "123",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for invalid --content JSON")
	}
	if !strings.Contains(err.Error(), "--content must be valid ContentBlock JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressCreateValidate_MissingTargetID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for missing --target-id")
	}
}

func TestProgressCreateValidate_InvalidTargetID_NonNumeric(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
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

func TestProgressCreateValidate_InvalidTargetType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
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

func TestProgressCreateValidate_ControlCharsInContent(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", "{\"blocks\":[{\"block_element_type\":\"para\tgraph\"}]}",
		"--target-id", "123",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for control chars in --content")
	}
}

func TestProgressCreateValidate_InvalidUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-id", "123",
		"--target-type", "objective",
		"--user-id-type", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid --user-id-type")
	}
}

func TestProgressCreateValidate_InvalidProgressPercent_OutOfRange(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-id", "123",
		"--target-type", "objective",
		"--progress-percent", "999999999999",
	})
	if err == nil {
		t.Fatal("expected error for --progress-percent > 100")
	}
	if !strings.Contains(err.Error(), "--progress-percent must be a number between -99999999999 and 99999999999") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressCreateValidate_InvalidProgressPercent_NonNumeric(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-id", "123",
		"--target-type", "objective",
		"--progress-percent", "abc",
	})
	if err == nil {
		t.Fatal("expected error for non-numeric --progress-percent")
	}
	if !strings.Contains(err.Error(), "--progress-percent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressCreateValidate_InvalidProgressStatus(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-id", "123",
		"--target-type", "objective",
		"--progress-status", "invalid_status",
	})
	if err == nil {
		t.Fatal("expected error for invalid --progress-status")
	}
	if !strings.Contains(err.Error(), "--progress-status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressCreateValidate_Valid(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v1/progress_records/",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"progress_id": "100",
				"modify_time": "1735776000000",
			},
		},
	})
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-id", "123",
		"--target-type", "objective",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- DryRun tests ---

func TestProgressCreateDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-id", "123",
		"--target-type", "objective",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v1/progress_records/") {
		t.Fatalf("dry-run output should contain API path, got: %s", output)
	}
	if !strings.Contains(output, "POST") {
		t.Fatalf("dry-run output should contain POST method, got: %s", output)
	}
	// Verify body contains content and target info
	if !strings.Contains(output, "target_id") {
		t.Fatalf("dry-run output should contain target_id, got: %s", output)
	}
	if !strings.Contains(output, "source_url") {
		t.Fatalf("dry-run output should contain source_url (brand default), got: %s", output)
	}
}

func TestProgressCreateDryRun_WithProgressRate(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-id", "123",
		"--target-type", "objective",
		"--progress-percent", "75",
		"--progress-status", "done",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "progress_rate") {
		t.Fatalf("dry-run output should contain progress_rate, got: %s", output)
	}
}

// --- Execute tests ---

func TestProgressCreateExecute_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v1/progress_records/",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"progress_id": "200",
				"modify_time": "1735776000000",
			},
		},
	})
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-id", "456",
		"--target-type", "key_result",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	pr, _ := data["progress"].(map[string]interface{})
	if pr == nil {
		t.Fatal("expected progress in output")
	}
	if pr["progress_id"] != "200" {
		t.Fatalf("progress_id = %v, want 200", pr["progress_id"])
	}
}

func TestProgressCreateExecute_APIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressCreateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v1/progress_records/",
		Status: 400,
		Body: map[string]interface{}{
			"code": 1001001,
			"msg":  "invalid parameters",
		},
	})
	err := runProgressCreateShortcut(t, f, stdout, []string{
		"+progress-create",
		"--content", validContentBlockJSON,
		"--target-id", "789",
		"--target-type", "objective",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}
