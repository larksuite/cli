// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

func batchCreateTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-batch-create",
		AppSecret: "secret-okr-batch-create",
		Brand:     core.BrandFeishu,
	}
}

func runBatchCreateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRBatchCreate.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

const validBatchCreateInput = `[
  {"text":"Objective 1","mention":["ou_123"],"krs":[{"text":"KR 1.1","mention":["ou_456"]}]},
  {"text":"Objective 2","krs":[{"text":"KR 2.1"},{"text":"KR 2.2"}]}
]`

// --- Validate tests ---

func TestBatchCreateValidate_MissingCycleID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--input", validBatchCreateInput,
	})
	// cobra Required:true reports flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "cycle-id") {
		t.Fatalf("expected --cycle-id required error, got: %v", err)
	}
}

func TestBatchCreateValidate_InvalidCycleID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "abc",
		"--input", validBatchCreateInput,
	})
	if err == nil {
		t.Fatal("expected error for invalid --cycle-id")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--cycle-id" {
		t.Fatalf("expected param --cycle-id, got %q", validationErr.Param)
	}
}

func TestBatchCreateValidate_MissingInput(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
	})
	// cobra Required:true reports flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "input") {
		t.Fatalf("expected --input required error, got: %v", err)
	}
}

func TestBatchCreateValidate_InvalidInputJSON(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid --input JSON")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--input" {
		t.Fatalf("expected param --input, got %q", validationErr.Param)
	}
}

func TestBatchCreateValidate_EmptyInputArray(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", "[]",
	})
	if err == nil {
		t.Fatal("expected error for empty --input array")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--input" {
		t.Fatalf("expected param --input, got %q", validationErr.Param)
	}
}

func TestBatchCreateValidate_EmptyObjectiveText(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", `[{"text":"","krs":[{"text":"KR 1"}]}]`,
	})
	if err == nil {
		t.Fatal("expected error for empty objective text")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--input" {
		t.Fatalf("expected param --input, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "objective[0].text") {
		t.Fatalf("expected error to mention objective[0].text, got: %v", err)
	}
}

func TestBatchCreateValidate_EmptyKRText(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", `[{"text":"Obj 1","krs":[{"text":""}]}]`,
	})
	if err == nil {
		t.Fatal("expected error for empty KR text")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--input" {
		t.Fatalf("expected param --input, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "objective[0].krs[0].text") {
		t.Fatalf("expected error to mention objective[0].krs[0].text, got: %v", err)
	}
}

func TestBatchCreateValidate_InvalidUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", validBatchCreateInput,
		"--user-id-type", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid --user-id-type")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--user-id-type" {
		t.Fatalf("expected param --user-id-type, got %q", validationErr.Param)
	}
}

func TestBatchCreateValidate_Valid(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"objective_id": "100",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/objectives/100/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"key_result_id": "200",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"objective_id": "101",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/objectives/101/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"key_result_id": "201",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/objectives/101/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"key_result_id": "202",
			},
		},
	})
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", validBatchCreateInput,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- DryRun tests ---

func TestBatchCreateDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", validBatchCreateInput,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v2/cycles/123/objectives") {
		t.Fatalf("dry-run output should contain objective creation API path, got: %s", output)
	}
	if !strings.Contains(output, "POST") {
		t.Fatalf("dry-run output should contain POST method, got: %s", output)
	}
	if !strings.Contains(output, "/open-apis/okr/v2/objectives/") || !strings.Contains(output, "/key_results") {
		t.Fatalf("dry-run output should contain KR creation API path, got: %s", output)
	}
	// Verify content is in the body
	if !strings.Contains(output, "Objective 1") {
		t.Fatalf("dry-run output should contain objective text, got: %s", output)
	}
	if !strings.Contains(output, "KR 1.1") {
		t.Fatalf("dry-run output should contain KR text, got: %s", output)
	}
}

// --- Execute tests ---

func TestBatchCreateExecute_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"objective_id": "100",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/objectives/100/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"key_result_id": "200",
			},
		},
	})
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", `[{"text":"Obj 1","krs":[{"text":"KR 1"}]}]`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output
	data := decodeEnvelope(t, stdout)
	ok, _ := data["ok"].(bool)
	if !ok {
		t.Fatal("expected ok=true in output")
	}
	dataField, _ := data["data"].(map[string]interface{})
	created, _ := dataField["created"].([]interface{})
	if len(created) != 1 {
		t.Fatalf("expected 1 created objective, got %d", len(created))
	}
	obj, _ := created[0].(map[string]interface{})
	if obj["objective_id"] != "100" {
		t.Fatalf("expected objective_id=100, got %v", obj["objective_id"])
	}
	krs, _ := obj["krs"].([]interface{})
	if len(krs) != 1 || krs[0] != "200" {
		t.Fatalf("expected krs=[200], got %v", krs)
	}
}

func TestBatchCreateExecute_APIErrorOnObjective(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Status: 400,
		Body: map[string]interface{}{
			"code": 1001001,
			"msg":  "invalid parameters",
		},
	})
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", `[{"text":"Obj 1"}]`,
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	// Should be a typed error from the API
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if prob.Category != "api" {
		t.Fatalf("expected api category, got %q", prob.Category)
	}
}

func TestBatchCreateExecute_APIErrorOnKR_TriggersRollback(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	// First objective creation succeeds
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"objective_id": "100",
			},
		},
	})
	// KR creation fails
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/objectives/100/key_results",
		Status: 400,
		Body: map[string]interface{}{
			"code": 1001001,
			"msg":  "invalid parameters",
		},
	})
	// Rollback: delete the created objective
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/okr/v2/objectives/100",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"objective_id": "100",
			},
		},
	})
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", `[{"text":"Obj 1","krs":[{"text":"KR 1"}]}]`,
	})
	if err == nil {
		t.Fatal("expected error for KR creation failure")
	}
	// Error should mention rollback
	if !strings.Contains(err.Error(), "rolling back") && !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("expected error to mention rollback, got: %v", err)
	}
	// Assert typed error metadata
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if prob.Category != errs.CategoryAPI {
		t.Fatalf("expected api category (preserved from original error), got %q", prob.Category)
	}
	// Assert cause preservation
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected error to wrap APIError, got: %T", err)
	}
	if !errors.Is(err, apiErr) {
		t.Fatal("expected errors.Is to find the wrapped APIError")
	}
}

func TestBatchCreateExecute_RollbackDeleteFails(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, batchCreateTestConfig(t))
	// Objective creation succeeds
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"objective_id": "100",
			},
		},
	})
	// KR creation fails
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/objectives/100/key_results",
		Status: 400,
		Body: map[string]interface{}{
			"code": 1001001,
			"msg":  "invalid parameters",
		},
	})
	// Rollback delete also fails
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/okr/v2/objectives/100",
		Status: 500,
		Body: map[string]interface{}{
			"code": 9999999,
			"msg":  "internal error",
		},
	})
	err := runBatchCreateShortcut(t, f, stdout, []string{
		"+batch-create",
		"--cycle-id", "123",
		"--input", `[{"text":"Obj 1","krs":[{"text":"KR 1"}]}]`,
	})
	if err == nil {
		t.Fatal("expected error for KR creation failure")
	}
	// Error should mention residual resources
	if !strings.Contains(err.Error(), "residual") && !strings.Contains(err.Error(), "manual cleanup") {
		t.Fatalf("expected error to mention residual resources, got: %v", err)
	}
	if !strings.Contains(err.Error(), "objective:100") {
		t.Fatalf("expected error to list residual objective ID, got: %v", err)
	}
}

// --- Unit tests for helper functions ---

func TestParseBatchCreateInput_Valid(t *testing.T) {
	t.Parallel()
	input := `[{"text":"Obj 1","mention":["ou_123"],"krs":[{"text":"KR 1","mention":["ou_456"]}]}]`
	objs, err := parseBatchCreateInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 objective, got %d", len(objs))
	}
	if objs[0].Text != "Obj 1" {
		t.Fatalf("expected text 'Obj 1', got %q", objs[0].Text)
	}
	if len(objs[0].Mention) != 1 || objs[0].Mention[0] != "ou_123" {
		t.Fatalf("expected mention ['ou_123'], got %v", objs[0].Mention)
	}
	if len(objs[0].KRs) != 1 {
		t.Fatalf("expected 1 KR, got %d", len(objs[0].KRs))
	}
	if objs[0].KRs[0].Text != "KR 1" {
		t.Fatalf("expected KR text 'KR 1', got %q", objs[0].KRs[0].Text)
	}
}

func TestBuildContentBlock(t *testing.T) {
	t.Parallel()
	cb := buildContentBlock("Test text", []string{"ou_123", "ou_456"})
	if cb == nil {
		t.Fatal("expected non-nil ContentBlock")
	}
	if len(cb.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(cb.Blocks))
	}
	block := cb.Blocks[0]
	if block.BlockElementType == nil || *block.BlockElementType != BlockElementTypeParagraph {
		t.Fatalf("expected paragraph block type")
	}
	if block.Paragraph == nil {
		t.Fatal("expected non-nil paragraph")
	}
	// Should have 3 elements: 1 text + 2 mentions
	if len(block.Paragraph.Elements) != 3 {
		t.Fatalf("expected 3 paragraph elements, got %d", len(block.Paragraph.Elements))
	}
	// First element should be textRun
	if block.Paragraph.Elements[0].ParagraphElementType == nil ||
		*block.Paragraph.Elements[0].ParagraphElementType != ParagraphElementTypeTextRun {
		t.Fatal("expected first element to be textRun")
	}
	if block.Paragraph.Elements[0].TextRun == nil || *block.Paragraph.Elements[0].TextRun.Text != "Test text" {
		t.Fatalf("expected text 'Test text', got %v", block.Paragraph.Elements[0].TextRun)
	}
	// Second and third should be mentions
	for i := 1; i <= 2; i++ {
		if block.Paragraph.Elements[i].ParagraphElementType == nil ||
			*block.Paragraph.Elements[i].ParagraphElementType != ParagraphElementTypeMention {
			t.Fatalf("expected element %d to be mention", i)
		}
	}
}
