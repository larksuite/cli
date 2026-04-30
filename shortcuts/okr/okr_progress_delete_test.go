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

func progressDeleteTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-progress-delete",
		AppSecret: "secret-okr-progress-delete",
		Brand:     core.BrandFeishu,
	}
}

func runProgressDeleteShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRDeleteProgressRecord.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestProgressDeleteValidate_MissingProgressID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressDeleteTestConfig(t))
	err := runProgressDeleteShortcut(t, f, stdout, []string{"+progress-delete"})
	if err == nil {
		t.Fatal("expected error for missing --progress-id")
	}
}

func TestProgressDeleteValidate_InvalidProgressID_NonNumeric(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressDeleteTestConfig(t))
	err := runProgressDeleteShortcut(t, f, stdout, []string{"+progress-delete", "--progress-id", "abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric --progress-id")
	}
	if !strings.Contains(err.Error(), "--progress-id must be a positive int64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProgressDeleteValidate_InvalidProgressID_Zero(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressDeleteTestConfig(t))
	err := runProgressDeleteShortcut(t, f, stdout, []string{"+progress-delete", "--progress-id", "0"})
	if err == nil {
		t.Fatal("expected error for zero --progress-id")
	}
}

func TestProgressDeleteValidate_InvalidProgressID_Negative(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressDeleteTestConfig(t))
	err := runProgressDeleteShortcut(t, f, stdout, []string{"+progress-delete", "--progress-id", "-1"})
	if err == nil {
		t.Fatal("expected error for negative --progress-id")
	}
}

func TestProgressDeleteValidate_Valid(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressDeleteTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/okr/v1/progress_records/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})
	err := runProgressDeleteShortcut(t, f, stdout, []string{"+progress-delete", "--progress-id", "123", "--yes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- DryRun tests ---

func TestProgressDeleteDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, progressDeleteTestConfig(t))
	err := runProgressDeleteShortcut(t, f, stdout, []string{
		"+progress-delete",
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
	if !strings.Contains(output, "DELETE") {
		t.Fatalf("dry-run output should contain DELETE method, got: %s", output)
	}
}

// --- Execute tests ---

func TestProgressDeleteExecute_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressDeleteTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/okr/v1/progress_records/789",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})
	err := runProgressDeleteShortcut(t, f, stdout, []string{"+progress-delete", "--progress-id", "789", "--yes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	if data["deleted"] != true {
		t.Fatalf("deleted = %v, want true", data["deleted"])
	}
	if data["progress_id"] != "789" {
		t.Fatalf("progress_id = %v, want 789", data["progress_id"])
	}
}

func TestProgressDeleteExecute_APIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, progressDeleteTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/okr/v1/progress_records/999",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "internal error",
		},
	})
	err := runProgressDeleteShortcut(t, f, stdout, []string{"+progress-delete", "--progress-id", "999", "--yes"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}
