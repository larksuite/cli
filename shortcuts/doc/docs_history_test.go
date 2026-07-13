// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestDocsHistoryValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		shortcut  common.Shortcut
		args      []string
		param     string
		category  errs.Category
		subtype   errs.Subtype
		wantCause bool
	}{
		{
			name:     "list rejects legacy doc URL",
			shortcut: DocsHistoryList,
			args:     []string{"+history-list", "--doc", "https://example.feishu.cn/doc/old_doc", "--as", "bot"},
			param:    "--doc",
			category: errs.CategoryValidation,
			subtype:  errs.SubtypeInvalidArgument,
		},
		{
			name:     "list rejects invalid page size",
			shortcut: DocsHistoryList,
			args:     []string{"+history-list", "--doc", "doxcnHistory", "--page-size", "0", "--as", "bot"},
			param:    "--page-size",
			category: errs.CategoryValidation,
			subtype:  errs.SubtypeInvalidArgument,
		},
		{
			name:      "revert rejects non-numeric history version id",
			shortcut:  DocsHistoryRevert,
			args:      []string{"+history-revert", "--doc", "doxcnHistory", "--history-version-id", "abc", "--as", "bot"},
			param:     "--history-version-id",
			category:  errs.CategoryValidation,
			subtype:   errs.SubtypeInvalidArgument,
			wantCause: true,
		},
		{
			name:     "revert rejects non-positive history version id",
			shortcut: DocsHistoryRevert,
			args:     []string{"+history-revert", "--doc", "doxcnHistory", "--history-version-id", "0", "--as", "bot"},
			param:    "--history-version-id",
			category: errs.CategoryValidation,
			subtype:  errs.SubtypeInvalidArgument,
		},
		{
			name:     "revert rejects invalid wait timeout",
			shortcut: DocsHistoryRevert,
			args:     []string{"+history-revert", "--doc", "doxcnHistory", "--history-version-id", "10", "--wait-timeout-ms", "30001", "--as", "bot"},
			param:    "--wait-timeout-ms",
			category: errs.CategoryValidation,
			subtype:  errs.SubtypeInvalidArgument,
		},
		{
			name:     "status rejects empty task id",
			shortcut: DocsHistoryRevertStatus,
			args:     []string{"+history-revert-status", "--doc", "doxcnHistory", "--task-id", "", "--as", "bot"},
			param:    "--task-id",
			category: errs.CategoryValidation,
			subtype:  errs.SubtypeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-history-validation"))
			err := mountAndRunDocs(t, tt.shortcut, tt.args, f, stdout)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error is not typed: %T %v", err, err)
			}
			if problem.Category != tt.category {
				t.Fatalf("category = %q, want %q (err: %v)", problem.Category, tt.category, err)
			}
			if problem.Subtype != tt.subtype {
				t.Fatalf("subtype = %q, want %q (err: %v)", problem.Subtype, tt.subtype, err)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected validation error, got %T: %v", err, err)
			}
			if validationErr.Param != tt.param {
				t.Fatalf("param = %q, want %q (err: %v)", validationErr.Param, tt.param, err)
			}
			if tt.wantCause && errors.Unwrap(err) == nil {
				t.Fatalf("expected wrapped cause, got nil (err: %v)", err)
			}
		})
	}
}

func TestDocsHistoryDryRun(t *testing.T) {
	t.Parallel()

	listCmd := newDocsHistoryRuntimeCmd(t, DocsHistoryList, map[string]string{
		"doc":        "doxcnHistoryDryRun",
		"page-size":  "5",
		"page-token": "page_token_1",
	})
	listDry := decodeDocDryRun(t, DocsHistoryList.DryRun(context.Background(), common.TestNewRuntimeContext(listCmd, nil)))
	if got, want := listDry.API[0].URL, "/open-apis/docs_ai/v1/documents/doxcnHistoryDryRun/histories"; got != want {
		t.Fatalf("list dry-run URL = %q, want %q", got, want)
	}
	if got := int(listDry.API[0].Params["page_size"].(float64)); got != 5 {
		t.Fatalf("list page_size = %d, want 5", got)
	}
	if got := listDry.API[0].Params["page_token"]; got != "page_token_1" {
		t.Fatalf("list page_token = %#v, want page_token_1", got)
	}

	revertCmd := newDocsHistoryRuntimeCmd(t, DocsHistoryRevert, map[string]string{
		"doc":                "doxcnHistoryDryRun",
		"history-version-id": "42",
		"wait-timeout-ms":    "30000",
	})
	revertDry := decodeDocDryRun(t, DocsHistoryRevert.DryRun(context.Background(), common.TestNewRuntimeContext(revertCmd, nil)))
	if got, want := revertDry.API[0].URL, "/open-apis/docs_ai/v1/documents/doxcnHistoryDryRun/history/revert"; got != want {
		t.Fatalf("revert dry-run URL = %q, want %q", got, want)
	}
	if got := revertDry.API[0].Body["history_version_id"]; got != "42" {
		t.Fatalf("revert history_version_id = %#v, want 42", got)
	}
	if got := int(revertDry.API[0].Body["wait_timeout_ms"].(float64)); got != 30000 {
		t.Fatalf("revert wait_timeout_ms = %d, want 30000", got)
	}

	statusCmd := newDocsHistoryRuntimeCmd(t, DocsHistoryRevertStatus, map[string]string{
		"doc":     "doxcnHistoryDryRun",
		"task-id": "task_1",
	})
	statusDry := decodeDocDryRun(t, DocsHistoryRevertStatus.DryRun(context.Background(), common.TestNewRuntimeContext(statusCmd, nil)))
	if got, want := statusDry.API[0].URL, "/open-apis/docs_ai/v1/documents/doxcnHistoryDryRun/history/revert_status"; got != want {
		t.Fatalf("status dry-run URL = %q, want %q", got, want)
	}
	if got := statusDry.API[0].Params["task_id"]; got != "task_1" {
		t.Fatalf("status task_id = %#v, want task_1", got)
	}
}

func TestDocsHistoryExecuteList(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-history-list"))
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnHistory/histories",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"revision_id":        float64(42),
						"history_version_id": "11",
						"edit_time":          "1780000000",
						"type":               float64(1),
						"editor_ids":         []interface{}{"ou_1"},
					},
				},
				"has_more":   true,
				"page_token": "page_token_2",
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDocs(t, DocsHistoryList, []string{
		"+history-list",
		"--doc", "doxcnHistory",
		"--page-size", "5",
		"--page-token", "page_token_1",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsHistoryEnvelope(t, stdout)
	if got := data["page_token"]; got != "page_token_2" {
		t.Fatalf("page_token = %#v, want page_token_2", got)
	}
	entries, _ := data["entries"].([]interface{})
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want one entry", data["entries"])
	}
}

func TestDocsHistoryExecuteRevert(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-history-revert"))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnHistory/history/revert",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"task_id":            "task_1",
				"status":             "running",
				"history_version_id": "42",
				"poll_after_ms":      float64(10000),
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDocs(t, DocsHistoryRevert, []string{
		"+history-revert",
		"--doc", "doxcnHistory",
		"--history-version-id", "42",
		"--wait-timeout-ms", "0",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode revert body: %v\nraw=%s", err, stub.CapturedBody)
	}
	if got := body["history_version_id"]; got != "42" {
		t.Fatalf("history_version_id = %#v, want 42", got)
	}
	if got := int(body["wait_timeout_ms"].(float64)); got != 0 {
		t.Fatalf("wait_timeout_ms = %d, want 0", got)
	}

	data := decodeDocsHistoryEnvelope(t, stdout)
	if got := data["task_id"]; got != "task_1" {
		t.Fatalf("task_id = %#v, want task_1", got)
	}
}

func TestDocsHistoryExecuteRevertStatus(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-history-status"))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnHistory/history/revert_status",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"status":              "partial_failed",
				"history_version_id":  "11",
				"failed_block_tokens": []interface{}{"blk_1"},
			},
		},
	})

	err := mountAndRunDocs(t, DocsHistoryRevertStatus, []string{
		"+history-revert-status",
		"--doc", "doxcnHistory",
		"--task-id", "task_1",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsHistoryEnvelope(t, stdout)
	if got := data["status"]; got != "partial_failed" {
		t.Fatalf("status = %#v, want partial_failed", got)
	}
	if got := data["history_version_id"]; got != "11" {
		t.Fatalf("history_version_id = %#v, want 11", got)
	}
	failed, _ := data["failed_block_tokens"].([]interface{})
	if len(failed) != 1 || failed[0] != "blk_1" {
		t.Fatalf("failed_block_tokens = %#v, want [blk_1]", data["failed_block_tokens"])
	}
}

func newDocsHistoryRuntimeCmd(t *testing.T, shortcut common.Shortcut, values map[string]string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: shortcut.Command}
	for _, flag := range shortcut.Flags {
		switch flag.Type {
		case "int":
			cmd.Flags().Int(flag.Name, 0, flag.Desc)
		default:
			cmd.Flags().String(flag.Name, flag.Default, flag.Desc)
		}
	}
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return cmd
}

func decodeDocsHistoryEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in envelope: %#v", envelope)
	}
	return data
}

func TestDocsHistoryURLValidationMessage(t *testing.T) {
	t.Parallel()

	_, err := parseDocsHistoryDocRef("https://example.feishu.cn/doc/old_doc", "+history-list")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "only supports docx documents") {
		t.Fatalf("unexpected error: %v", err)
	}
}
