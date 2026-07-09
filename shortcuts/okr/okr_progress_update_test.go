// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func progressUpdateTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-progress-update",
		AppSecret: "secret-okr-progress-update",
		Brand:     core.BrandFeishu,
	}
}

func runProgressUpdateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRUpdateProgressRecord.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestProgressUpdateValidate_MissingProgressID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--content", validContentBlockJSON,
		"--style", "richtext",
	})
	if err == nil {
		t.Fatal("expected error for missing --progress-id")
	}
}

func TestProgressUpdateValidate_InvalidProgressID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "abc",
		"--content", validContentBlockJSON,
		"--style", "richtext",
	})
	if err == nil {
		t.Fatal("expected error for invalid --progress-id")
	}
	if !strings.Contains(err.Error(), "--progress-id must be a positive int64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressUpdateValidate_MissingContent(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "123",
	})
	if err == nil {
		t.Fatal("expected error for missing --content")
	}
}

func TestProgressUpdateValidate_InvalidContentJSON(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "123",
		"--content", "not-json",
		"--style", "richtext",
	})
	if err == nil {
		t.Fatal("expected error for invalid --content JSON")
	}
	if !strings.Contains(err.Error(), "--content must be valid ContentBlock JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressUpdateValidate_InvalidUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "123",
		"--content", validContentBlockJSON,
		"--style", "richtext",
		"--user-id-type", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid --user-id-type")
	}
}

func TestProgressUpdateValidate_InvalidProgressPercent_OutOfRange(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "123",
		"--content", validContentBlockJSON,
		"--style", "richtext",
		"--progress-percent", "-999999999999",
	})
	if err == nil {
		t.Fatal("expected error for negative --progress-percent")
	}
	if !strings.Contains(err.Error(), "--progress-percent must be a number between -99999999999 and 99999999999") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressUpdateValidate_InvalidProgressStatus(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "123",
		"--content", validContentBlockJSON,
		"--style", "richtext",
		"--progress-status", "invalid_status",
	})
	if err == nil {
		t.Fatal("expected error for invalid --progress-status")
	}
	if !strings.Contains(err.Error(), "--progress-status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressUpdateValidate_Valid(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PUT",
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
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "123",
		"--content", validContentBlockJSON,
		"--style", "richtext",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- DryRun tests ---

func TestProgressUpdateDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "456",
		"--content", validContentBlockJSON,
		"--style", "richtext",
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
	if !strings.Contains(output, "PUT") {
		t.Fatalf("dry-run output should contain PUT method, got: %s", output)
	}
}

func TestProgressUpdateDryRun_WithProgressRate(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "456",
		"--content", validContentBlockJSON,
		"--style", "richtext",
		"--progress-percent", "50",
		"--progress-status", "overdue",
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

func TestProgressUpdateExecute_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/okr/v1/progress_records/789",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"progress_id": "789",
				"modify_time": "1735776000000",
			},
		},
	})
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "789",
		"--content", validContentBlockJSON,
		"--style", "richtext",
	})
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

func TestProgressUpdateExecute_APIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/okr/v1/progress_records/999",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "internal error",
		},
	})
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "999",
		"--content", validContentBlockJSON,
		"--style", "richtext",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// --- Simple mode tests ---

func TestProgressUpdateExecute_SimpleMode_DefaultStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/okr/v1/progress_records/500",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"progress_id": "500",
				"modify_time": "1735776000000",
			},
		},
	})
	// Use default style (simple) without specifying --style
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "500",
		"--content", validSemiPlainJSON,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	pr, _ := data["progress"].(map[string]interface{})
	if pr == nil {
		t.Fatal("expected progress in output")
	}
	if pr["progress_id"] != "500" {
		t.Fatalf("progress_id = %v, want 500", pr["progress_id"])
	}
}

func TestProgressUpdateExecute_SimpleMode_ExplicitStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/okr/v1/progress_records/600",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"progress_id": "600",
				"modify_time": "1735776000000",
			},
		},
	})
	// Explicitly specify --style simple with mentions and progress rate
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "600",
		"--content", `{"text":"updated progress","mention":["ou_abc"]}`,
		"--style", "simple",
		"--progress-percent", "80",
		"--progress-status", "normal",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	pr, _ := data["progress"].(map[string]interface{})
	if pr == nil {
		t.Fatal("expected progress in output")
	}
	if pr["progress_id"] != "600" {
		t.Fatalf("progress_id = %v, want 600", pr["progress_id"])
	}
}

func TestProgressUpdateValidate_SimpleMode_InvalidSemiPlainJSON(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "123",
		"--content", `{"text":"invalid json`,
	})
	if err == nil {
		t.Fatal("expected error for invalid semi-plain JSON")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--content" {
		t.Fatalf("expected param %q, got %q", "--content", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "--content must be valid semi-plain JSON") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressUpdateValidate_SimpleMode_EmptyMention(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "123",
		"--content", `{"text":"has empty mention","mention":["ou_abc",""]}`,
	})
	if err == nil {
		t.Fatal("expected error for empty mention in simple mode")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--content" {
		t.Fatalf("expected param %q, got %q", "--content", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "--content mention[1] cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressUpdateValidate_SimpleMode_ImagesNotSupported(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "123",
		"--content", `{"text":"has images","mention":[],"images":["img_token"]}`,
	})
	if err == nil {
		t.Fatal("expected error for images in simple mode")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--content" {
		t.Fatalf("expected param %q, got %q", "--content", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "docs and images are not supported in simple style input") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressUpdateDryRun_SimpleMode(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressUpdateTestConfig(t))
	err := runProgressUpdateShortcut(t, f, stdout, []string{
		"+progress-update",
		"--progress-id", "700",
		"--content", validSemiPlainJSON,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v1/progress_records/700") {
		t.Fatalf("dry-run output should contain API path, got: %s", output)
	}
	if !strings.Contains(output, "PUT") {
		t.Fatalf("dry-run output should contain PUT method, got: %s", output)
	}
}
