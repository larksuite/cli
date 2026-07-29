// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestSlidesHistoryDeclaredScopes(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		wantBase []string
		wantFull []string
	}{
		{
			name:     "list",
			shortcut: SlidesHistoryList,
			wantBase: []string{"slides:presentation:read"},
			wantFull: []string{"slides:presentation:read", "wiki:node:read"},
		},
		{
			name:     "revert",
			shortcut: SlidesHistoryRevert,
			wantBase: []string{"slides:presentation:update", "slides:presentation:write_only"},
			wantFull: []string{"slides:presentation:update", "slides:presentation:write_only", "wiki:node:read"},
		},
		{
			name:     "status",
			shortcut: SlidesHistoryRevertStatus,
			wantBase: []string{"slides:presentation:read"},
			wantFull: []string{"slides:presentation:read", "wiki:node:read"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.shortcut.ScopesForIdentity("user"); !reflect.DeepEqual(got, tt.wantBase) {
				t.Fatalf("user preflight scopes = %#v, want %#v", got, tt.wantBase)
			}
			if got := tt.shortcut.ScopesForIdentity("bot"); !reflect.DeepEqual(got, tt.wantBase) {
				t.Fatalf("bot preflight scopes = %#v, want %#v", got, tt.wantBase)
			}
			if got := tt.shortcut.DeclaredScopesForIdentity("user"); !reflect.DeepEqual(got, tt.wantFull) {
				t.Fatalf("declared scopes = %#v, want %#v", got, tt.wantFull)
			}
		})
	}
}

func TestSlidesHistoryValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		shortcut  common.Shortcut
		args      []string
		param     string
		wantCause bool
	}{
		{
			name:     "list rejects unsupported presentation input",
			shortcut: SlidesHistoryList,
			args:     []string{"+history-list", "--presentation", "tmp/wiki/wikcn123", "--as", "bot"},
			param:    "--presentation",
		},
		{
			name:     "list rejects invalid page size",
			shortcut: SlidesHistoryList,
			args:     []string{"+history-list", "--presentation", "presHistory", "--page-size", "0", "--as", "bot"},
			param:    "--page-size",
		},
		{
			name:      "revert rejects non-numeric history version id",
			shortcut:  SlidesHistoryRevert,
			args:      []string{"+history-revert", "--presentation", "presHistory", "--history-version-id", "abc", "--as", "bot"},
			param:     "--history-version-id",
			wantCause: true,
		},
		{
			name:     "revert rejects non-positive history version id",
			shortcut: SlidesHistoryRevert,
			args:     []string{"+history-revert", "--presentation", "presHistory", "--history-version-id", "0", "--as", "bot"},
			param:    "--history-version-id",
		},
		{
			name:     "status rejects empty task id",
			shortcut: SlidesHistoryRevertStatus,
			args:     []string{"+history-revert-status", "--presentation", "presHistory", "--task-id", "", "--as", "bot"},
			param:    "--task-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			err := runSlidesShortcut(t, f, stdout, tt.shortcut, tt.args)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			_, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error is not typed: %T %v", err, err)
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

func TestSlidesHistoryDryRun(t *testing.T) {
	t.Parallel()

	listCmd := newSlidesHistoryRuntimeCmd(t, SlidesHistoryList, map[string]string{
		"presentation": "presHistoryDryRun",
		"page-size":    "5",
		"page-token":   "page_token_1",
	})
	listDry := decodeSlidesHistoryDryRun(t, SlidesHistoryList.DryRun(context.Background(), common.TestNewRuntimeContext(listCmd, nil)))
	if got, want := listDry.API[0].URL, "/open-apis/slides_ai/v1/xml_presentations/presHistoryDryRun/histories"; got != want {
		t.Fatalf("list dry-run URL = %q, want %q", got, want)
	}
	if got := int(listDry.API[0].Params["page_size"].(float64)); got != 5 {
		t.Fatalf("list page_size = %d, want 5", got)
	}
	if got := listDry.API[0].Params["page_token"]; got != "page_token_1" {
		t.Fatalf("list page_token = %#v, want page_token_1", got)
	}

	revertCmd := newSlidesHistoryRuntimeCmd(t, SlidesHistoryRevert, map[string]string{
		"presentation":       "presHistoryDryRun",
		"history-version-id": "42",
	})
	revertDry := decodeSlidesHistoryDryRun(t, SlidesHistoryRevert.DryRun(context.Background(), common.TestNewRuntimeContext(revertCmd, nil)))
	if got, want := revertDry.API[0].URL, "/open-apis/slides_ai/v1/xml_presentations/presHistoryDryRun/history/revert"; got != want {
		t.Fatalf("revert dry-run URL = %q, want %q", got, want)
	}
	if got := revertDry.API[0].Body["history_version_id"]; got != "42" {
		t.Fatalf("revert history_version_id = %#v, want 42", got)
	}
	if _, ok := revertDry.API[0].Body["wait_timeout_ms"]; ok {
		t.Fatal("revert body must not contain wait_timeout_ms")
	}

	statusCmd := newSlidesHistoryRuntimeCmd(t, SlidesHistoryRevertStatus, map[string]string{
		"presentation": "presHistoryDryRun",
		"task-id":      "task_1",
	})
	statusDry := decodeSlidesHistoryDryRun(t, SlidesHistoryRevertStatus.DryRun(context.Background(), common.TestNewRuntimeContext(statusCmd, nil)))
	if got, want := statusDry.API[0].URL, "/open-apis/slides_ai/v1/xml_presentations/presHistoryDryRun/history/revert_status"; got != want {
		t.Fatalf("status dry-run URL = %q, want %q", got, want)
	}
	if got := statusDry.API[0].Params["task_id"]; got != "task_1" {
		t.Fatalf("status task_id = %#v, want task_1", got)
	}
}

func TestSlidesHistoryDryRunWithWikiPresentation(t *testing.T) {
	t.Parallel()

	cmd := newSlidesHistoryRuntimeCmd(t, SlidesHistoryList, map[string]string{
		"presentation": "https://example.feishu.cn/wiki/wikcn123",
		"page-size":    "20",
	})
	dry := decodeSlidesHistoryDryRun(t, SlidesHistoryList.DryRun(context.Background(), common.TestNewRuntimeContext(cmd, nil)))
	if len(dry.API) != 2 {
		t.Fatalf("api calls = %d, want 2: %#v", len(dry.API), dry.API)
	}
	if got, want := dry.API[0].URL, "/open-apis/wiki/v2/spaces/get_node"; got != want {
		t.Fatalf("wiki dry-run URL = %q, want %q", got, want)
	}
	if got := dry.API[0].Params["token"]; got != "wikcn123" {
		t.Fatalf("wiki node parameter mismatch: got %#v, want placeholder node id", got)
	}
	if got, want := dry.API[1].URL, "/open-apis/slides_ai/v1/xml_presentations/%3Cresolved_slides_token%3E/histories"; got != want {
		t.Fatalf("history dry-run URL = %q, want %q", got, want)
	}
}

func TestSlidesHistoryExecuteList(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var capturedQuery url.Values
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/presHistory/histories",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entries": []interface{}{
					map[string]interface{}{
						"revision_id":        float64(42),
						"history_version_id": "11",
						"edit_time":          "2026-06-22T12:24:45Z",
						"type":               float64(1),
						"editor_ids":         []interface{}{"ou_1"},
					},
				},
				"has_more":   true,
				"page_token": "page_token_2",
			},
		},
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.Query()
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesHistoryList, []string{
		"+history-list",
		"--presentation", "presHistory",
		"--page-size", "5",
		"--page-token", "page_token_1",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := capturedQuery.Get("page_size"); got != "5" {
		t.Fatalf("page_size query = %q, want 5", got)
	}
	if got := capturedQuery.Get("page_token"); got != "page_token_1" {
		t.Fatalf("page_token query = %q, want page_token_1", got)
	}

	data := decodeSlidesHistoryEnvelope(t, stdout)
	if got := data["page_token"]; got != "page_token_2" {
		t.Fatalf("page_token = %#v, want page_token_2", got)
	}
	entries, _ := data["entries"].([]interface{})
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want one entry", data["entries"])
	}
}

func TestSlidesHistoryExecuteRevert(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/presHistory/history/revert",
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

	err := runSlidesShortcut(t, f, stdout, SlidesHistoryRevert, []string{
		"+history-revert",
		"--presentation", "presHistory",
		"--history-version-id", "42",
		"--as", "bot",
	})
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
	if _, ok := body["wait_timeout_ms"]; ok {
		t.Fatal("revert body must not contain wait_timeout_ms")
	}

	data := decodeSlidesHistoryEnvelope(t, stdout)
	if got := data["task_id"]; got != "task_1" {
		t.Fatalf("task_id = %#v, want task_1", got)
	}
}

func TestSlidesHistoryExecuteRevertStatus(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var capturedQuery url.Values
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/presHistory/history/revert_status",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"status":             "done",
				"history_version_id": "11",
			},
		},
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.Query()
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesHistoryRevertStatus, []string{
		"+history-revert-status",
		"--presentation", "presHistory",
		"--task-id", "task_1",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := capturedQuery.Get("task_id"); got != "task_1" {
		t.Fatalf("task_id query = %q, want task_1", got)
	}
	data := decodeSlidesHistoryEnvelope(t, stdout)
	if got := data["status"]; got != "done" {
		t.Fatalf("status = %#v, want done", got)
	}
	if got := data["history_version_id"]; got != "11" {
		t.Fatalf("history_version_id = %#v, want 11", got)
	}
}

func TestSlidesHistoryExecuteResolvesWikiPresentation(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "slides",
					"obj_token": "presReal",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/presReal/histories",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"entries":    []interface{}{},
				"has_more":   false,
				"page_token": "",
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesHistoryList, []string{
		"+history-list",
		"--presentation", "https://example.feishu.cn/wiki/wikcn123",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeSlidesHistoryEnvelope(t, stdout)
	if got := data["has_more"]; got != false {
		t.Fatalf("has_more = %#v, want false", got)
	}
}

type slidesHistoryDryRunOutput struct {
	API []struct {
		Method string                 `json:"method"`
		URL    string                 `json:"url"`
		Params map[string]interface{} `json:"params"`
		Body   map[string]interface{} `json:"body"`
	} `json:"api"`
}

func newSlidesHistoryRuntimeCmd(t *testing.T, shortcut common.Shortcut, values map[string]string) *cobra.Command {
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

func decodeSlidesHistoryDryRun(t *testing.T, dry *common.DryRunAPI) slidesHistoryDryRunOutput {
	t.Helper()

	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	var out slidesHistoryDryRunOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode dry-run: %v\nraw=%s", err, raw)
	}
	return out
}

func decodeSlidesHistoryEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
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
