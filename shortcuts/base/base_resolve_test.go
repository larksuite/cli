// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestBaseURLResolveBaseURL(t *testing.T) {
	t.Run("with coordinates", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(baseBlockListResolveStub("bas123",
			map[string]interface{}{"id": "tbl123", "type": "table", "name": "Orders"},
		))
		reg.Register(fieldListStub("bas123", "tbl123"))
		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve",
			"--url", "https://example.larkoffice.com/base/bas123?table=tbl123&view=vew123&record=rec123",
			"--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}

		data := decodeBaseEnvelope(t, stdout)
		if data["input_type"] != "base_url" || data["base_token"] != "bas123" {
			t.Fatalf("unexpected output: %#v", data)
		}
		if data["block_id"] != "tbl123" || data["selection_source"] != "url_query" || data["block_type"] != "table" || data["table_id"] != "tbl123" || data["view_id"] != "vew123" || data["record_id"] != "rec123" {
			t.Fatalf("missing Base coordinates: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		fields, _ := hint["fields"].(map[string]interface{})
		if hint["next_step"] != nextStepRecordList || fields["total"] != float64(2) {
			t.Fatalf("unexpected hint: %#v", hint)
		}
	})

	t.Run("base only", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/base/bas123", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["input_type"] != "base_url" || data["base_token"] != "bas123" {
			t.Fatalf("unexpected output: %#v", data)
		}
		if _, ok := data["table_id"]; ok {
			t.Fatalf("table_id should be omitted for base-only URL: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		if hint["next_step"] != nextStepBaseBlockList {
			t.Fatalf("unexpected hint: %#v", hint)
		}
	})

	t.Run("unconfirmed selected block stays neutral", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/base/bas123?table=tbl123&view=vew_stale&record=rec_stale", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["base_token"] != "bas123" || data["block_id"] != "tbl123" {
			t.Fatalf("unexpected output: %#v", data)
		}
		if _, ok := data["table_id"]; ok {
			t.Fatalf("unconfirmed block must not be reported as a table: %#v", data)
		}
		if _, ok := data["view_id"]; ok {
			t.Fatalf("unconfirmed block must not expose table-only view_id: %#v", data)
		}
		if _, ok := data["record_id"]; ok {
			t.Fatalf("unconfirmed block must not expose table-only record_id: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		if !strings.Contains(hint["next_step"].(string), "+base-block-list") {
			t.Fatalf("unexpected hint: %#v", hint)
		}
		if _, ok := hint["fields"]; ok {
			t.Fatalf("fields should be omitted when enrichment fails: %#v", hint)
		}
	})

	t.Run("field endpoint does not confirm untyped block", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(baseBlockListResolveStub("bas123",
			map[string]interface{}{"id": "tbl_other", "type": "table", "name": "Other"},
		))
		fieldStub := fieldListStub("bas123", "tbl123")
		fieldStub.Optional = true
		fieldStub.OnMatch = func(_ *http.Request) {
			t.Fatalf("field endpoint must not be used to infer selected block type")
		}
		reg.Register(fieldStub)
		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/base/bas123?table=tbl123&view=vew_stale", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["block_id"] != "tbl123" {
			t.Fatalf("unexpected block coordinates: %#v", data)
		}
		if _, ok := data["block_type"]; ok {
			t.Fatalf("field endpoint must not confirm block type without block directory: %#v", data)
		}
		if _, ok := data["table_id"]; ok {
			t.Fatalf("field endpoint must not promote an untyped block to table_id: %#v", data)
		}
		if _, ok := data["view_id"]; ok {
			t.Fatalf("untyped block must not expose table-only view_id: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		if _, ok := hint["fields"]; ok {
			t.Fatalf("fields should be omitted when block type is unconfirmed: %#v", hint)
		}
		if !strings.Contains(hint["next_step"].(string), "+base-block-list") {
			t.Fatalf("unexpected hint: %#v", hint)
		}
	})

	t.Run("dashboard selected through table query key", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(baseBlockListResolveStub("bas123",
			map[string]interface{}{"id": "blk_dashboard", "type": "dashboard", "name": "Sales"},
		))

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/base/bas123?table=blk_dashboard&view=vew_stale&record=rec_stale", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["block_id"] != "blk_dashboard" || data["selection_source"] != "url_query" || data["block_type"] != "dashboard" || data["dashboard_id"] != "blk_dashboard" || data["block_name"] != "Sales" {
			t.Fatalf("unexpected dashboard coordinates: %#v", data)
		}
		if _, ok := data["table_id"]; ok {
			t.Fatalf("dashboard must not be reported as table_id: %#v", data)
		}
		if _, ok := data["view_id"]; ok {
			t.Fatalf("dashboard must not expose table-only view_id: %#v", data)
		}
		if _, ok := data["record_id"]; ok {
			t.Fatalf("dashboard must not expose table-only record_id: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		nextStep := hint["next_step"].(string)
		if !strings.Contains(nextStep, "+dashboard-get") || !strings.Contains(nextStep, "+dashboard-list") || !strings.Contains(nextStep, "different dashboard than block_name") {
			t.Fatalf("unexpected dashboard hint: %#v", hint)
		}
	})

	t.Run("workflow selected through table query key", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(baseBlockListResolveStub("bas123",
			map[string]interface{}{"id": "wkf_notify", "type": "workflow", "name": "Notify"},
		))

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/base/bas123?table=wkf_notify&view=vew_stale&record=rec_stale", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["block_id"] != "wkf_notify" || data["block_type"] != "workflow" || data["workflow_id"] != "wkf_notify" {
			t.Fatalf("unexpected workflow coordinates: %#v", data)
		}
		if _, ok := data["table_id"]; ok {
			t.Fatalf("workflow must not be reported as table_id: %#v", data)
		}
		if _, ok := data["view_id"]; ok {
			t.Fatalf("workflow must not expose table-only view_id: %#v", data)
		}
		if _, ok := data["record_id"]; ok {
			t.Fatalf("workflow must not expose table-only record_id: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		if !strings.Contains(hint["next_step"].(string), "+workflow-get") {
			t.Fatalf("unexpected workflow hint: %#v", hint)
		}
	})

	t.Run("folder selected through table query key", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(baseBlockListResolveStub("bas123",
			map[string]interface{}{"id": "bfl_projects", "type": "folder", "name": "Projects"},
		))

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/base/bas123?table=bfl_projects&view=vew_stale&record=rec_stale", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["block_id"] != "bfl_projects" || data["block_type"] != "folder" || data["block_name"] != "Projects" {
			t.Fatalf("unexpected folder coordinates: %#v", data)
		}
		if _, ok := data["table_id"]; ok {
			t.Fatalf("folder must not be reported as table_id: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		nextStep := hint["next_step"].(string)
		if !strings.Contains(nextStep, "+base-block-list --base-token bas123 --parent-id bfl_projects") || strings.Contains(nextStep, "determine whether") {
			t.Fatalf("unexpected folder hint: %#v", hint)
		}
	})

	t.Run("docx selected through table query key", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(baseBlockListResolveStub("bas123",
			map[string]interface{}{"id": "blk_doc", "type": "docx", "name": "Spec", "docx_token": "docx123"},
		))

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/base/bas123?table=blk_doc&view=vew_stale&record=rec_stale", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["block_id"] != "blk_doc" || data["block_type"] != "docx" || data["block_name"] != "Spec" || data["docx_token"] != "docx123" {
			t.Fatalf("unexpected docx coordinates: %#v", data)
		}
		if _, ok := data["table_id"]; ok {
			t.Fatalf("docx must not be reported as table_id: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		nextStep := hint["next_step"].(string)
		if !strings.Contains(nextStep, "docs +fetch --doc docx123") || strings.Contains(nextStep, "determine whether") {
			t.Fatalf("unexpected docx hint: %#v", hint)
		}
	})
}

func baseBlockListResolveStub(baseToken string, blocks ...map[string]interface{}) *httpmock.Stub {
	items := make([]interface{}, 0, len(blocks))
	for _, block := range blocks {
		items = append(items, block)
	}
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/" + baseToken + "/blocks/list",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"blocks": items,
				"total":  len(items),
			},
		},
	}
}

func TestBaseURLResolveBaseAppURL(t *testing.T) {
	t.Run("with page and workspace coordinates", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve",
			"--url", "https://example.larkoffice.com/app/app_123?pre_pathname=%2Fbase%2Fworkspace%2Fws_123&pageId=pge_123",
			"--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}

		data := decodeBaseEnvelope(t, stdout)
		if data["input_type"] != "baseapp_url" || data["resource_type"] != "baseapp" {
			t.Fatalf("unexpected resource classification: %#v", data)
		}
		if data["app_token"] != "app_123" || data["workspace_token"] != "ws_123" || data["page_id"] != "pge_123" {
			t.Fatalf("missing BaseApp coordinates: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		if hint["next_step"] != nextStepBaseApp {
			t.Fatalf("unexpected hint: %#v", hint)
		}
	})

	t.Run("omits absent optional coordinates", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/app/app_123", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}

		data := decodeBaseEnvelope(t, stdout)
		if data["app_token"] != "app_123" {
			t.Fatalf("unexpected output: %#v", data)
		}
		if _, ok := data["workspace_token"]; ok {
			t.Fatalf("workspace_token should be omitted: %#v", data)
		}
		if _, ok := data["page_id"]; ok {
			t.Fatalf("page_id should be omitted: %#v", data)
		}
	})
}

func TestBaseURLResolveWikiURL(t *testing.T) {
	t.Run("bitable", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(wikiBaseNodeStub("wik123", "bas123", "Demo Base"))

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/wiki/wik123", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["input_type"] != "wiki_url" || data["base_token"] != "bas123" || data["title"] != "Demo Base" {
			t.Fatalf("unexpected output: %#v", data)
		}
	})

	t.Run("bitable with table coordinates", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(wikiBaseNodeStub("wik123", "bas123", "Demo Base"))
		reg.Register(baseBlockListResolveStub("bas123",
			map[string]interface{}{"id": "tbl123", "type": "table", "name": "Orders"},
		))
		reg.Register(fieldListStub("bas123", "tbl123"))

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve",
			"--url", "https://example.larkoffice.com/wiki/wik123?table=tbl123&view=vew123&record=rec123",
			"--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}

		data := decodeBaseEnvelope(t, stdout)
		if data["input_type"] != "wiki_url" || data["base_token"] != "bas123" || data["block_id"] != "tbl123" || data["block_type"] != "table" || data["table_id"] != "tbl123" || data["view_id"] != "vew123" || data["record_id"] != "rec123" {
			t.Fatalf("unexpected Wiki Base table coordinates: %#v", data)
		}
	})

	t.Run("bitable with dashboard selection", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(wikiBaseNodeStub("wik123", "bas123", "Demo Base"))
		reg.Register(baseBlockListResolveStub("bas123",
			map[string]interface{}{"id": "blk_dashboard", "type": "dashboard", "name": "Sales"},
		))

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve",
			"--url", "https://example.larkoffice.com/wiki/wik123?table=blk_dashboard&view=vew_stale&record=rec_stale",
			"--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}

		data := decodeBaseEnvelope(t, stdout)
		if data["input_type"] != "wiki_url" || data["block_id"] != "blk_dashboard" || data["block_type"] != "dashboard" || data["dashboard_id"] != "blk_dashboard" {
			t.Fatalf("unexpected Wiki Base dashboard coordinates: %#v", data)
		}
		if _, ok := data["view_id"]; ok {
			t.Fatalf("dashboard must not expose table-only view_id: %#v", data)
		}
		if _, ok := data["record_id"]; ok {
			t.Fatalf("dashboard must not expose table-only record_id: %#v", data)
		}
	})

	t.Run("non bitable", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/wiki/v2/spaces/get_node?token=wikdoc",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"node": map[string]interface{}{"obj_type": "docx", "obj_token": "docx123"},
				},
			},
		})

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/wiki/wikdoc", "--as", "user",
		}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "not Base") {
			t.Fatalf("err=%v, want non-Base validation error", err)
		}
	})
}

func wikiBaseNodeStub(wikiToken, baseToken, title string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node?token=" + wikiToken,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "bitable",
					"obj_token": baseToken,
					"title":     title,
				},
			},
		},
	}
}

func TestBaseURLResolveRecordShareURL(t *testing.T) {
	t.Run("enriched", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(recordShareMetaStub("shr123", "bas123", "tbl123", "rec123"))
		reg.Register(recordBatchGetStub("bas123", "tbl123", "rec123"))
		reg.Register(fieldListStub("bas123", "tbl123"))

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/record/shr123", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["input_type"] != "record_share_url" || data["base_token"] != "bas123" || data["record_id"] != "rec123" {
			t.Fatalf("unexpected output: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		recordData, _ := hint["record_data"].(map[string]interface{})
		fields, _ := hint["fields"].(map[string]interface{})
		nextStep, _ := hint["next_step"].(string)
		if !strings.Contains(nextStep, `+record-batch-update --base-token bas123 --table-id tbl123 --json '{"update_records":{"rec123":{"<field_id>":<CellValue>}}}'`) || recordData["fld_name"] != "Alice" || fields["total"] != float64(2) {
			t.Fatalf("unexpected hint: %#v", hint)
		}
	})

	t.Run("enrichment failure still returns meta", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(recordShareMetaStub("shr123", "bas123", "tbl123", "rec123"))

		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/record/shr123", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["input_type"] != "record_share_url" || data["base_token"] != "bas123" || data["record_id"] != "rec123" {
			t.Fatalf("unexpected output: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		nextStep, _ := hint["next_step"].(string)
		if !strings.Contains(nextStep, `+record-batch-update --base-token bas123 --table-id tbl123 --json '{"update_records":{"rec123":{"<field_id>":<CellValue>}}}'`) {
			t.Fatalf("unexpected hint: %#v", hint)
		}
		if _, ok := hint["record_data"]; ok {
			t.Fatalf("record_data should be omitted when enrichment fails: %#v", hint)
		}
		if _, ok := hint["fields"]; ok {
			t.Fatalf("fields should be omitted when enrichment fails: %#v", hint)
		}
	})
}

func recordShareMetaStub(shareToken, baseToken, tableID, recordID string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/record_share/" + shareToken + "/meta",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"record_share_token": shareToken,
				"base_token":         baseToken,
				"table_id":           tableID,
				"record_id":          recordID,
			},
		},
	}
}

func TestBaseURLResolveFormShareURL(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
		"+url-resolve", "--query", "https://example.larkoffice.com/share/base/form/shrform", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["input_type"] != "form_share_url" || data["share_token"] != "shrform" {
		t.Fatalf("unexpected output: %#v", data)
	}
}

func TestBaseURLResolveValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		wantText string
		wantHint string
	}{
		{"dashboard share", "https://example.larkoffice.com/share/base/dashboard/shr1", "CLI does not support resolving Base dashboard share URLs", "provide the URL of the Base itself"},
		{"view share", "https://example.larkoffice.com/share/base/view/shr1", "CLI does not support resolving Base view share URLs", "provide the URL of the Base itself"},
		{"workspace", "https://example.larkoffice.com/base/workspace/ws1", "CLI does not support resolving Base workspace URLs", "provide the URL of the Base itself"},
		{"add record", "https://example.larkoffice.com/base/add/addtoken", "CLI does not support resolving Base add-record URLs", "provide the URL of the Base itself"},
		{"unrelated", "https://example.larkoffice.com/docx/doc1", "not a supported Base URL pattern", ""},
		{"not url", "bas123", "only accepts full URLs", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
				"+url-resolve", "--url", tc.rawURL, "--as", "user",
			}, factory, stdout)
			if err == nil || !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("err=%v, want contains %q", err, tc.wantText)
			}
			p, ok := errs.ProblemOf(err)
			if !ok || p.Hint == "" {
				t.Fatalf("err=%v, want typed error with hint", err)
			}
			if tc.wantHint != "" && !strings.Contains(p.Hint, tc.wantHint) {
				t.Fatalf("hint=%q, want contains %q", p.Hint, tc.wantHint)
			}
			if strings.Contains(p.Hint, "original /base/{base_token}") {
				t.Fatalf("hint should not require original /base URL: %q", p.Hint)
			}
		})
	}
}

func TestBaseResolveAliasesUseCanonicalRepeatedFlagSemantics(t *testing.T) {
	t.Run("url resolve", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.com/base/bas1", "--query", "https://example.com/base/bas2", "--as", "user", "--dry-run",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, "bas2") || strings.Contains(got, "bas1") {
			t.Fatalf("alias should be the last occurrence: %s", got)
		}
	})

	t.Run("title resolve", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseTitleResolve, nil, []string{
			"+title-resolve", "--title", "Pipeline", "--query", "Sales", "--as", "user", "--dry-run",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, "Sales") || strings.Contains(got, "Pipeline") {
			t.Fatalf("alias should be the last occurrence: %s", got)
		}
	})
}

func TestBaseResolveHelpFlags(t *testing.T) {
	for _, tc := range []struct {
		shortcut    string
		definition  common.Shortcut
		primaryFlag string
		primaryDesc string
		aliasFlags  []string
	}{
		{
			shortcut:    "+url-resolve",
			definition:  BaseURLResolve,
			primaryFlag: "url",
			primaryDesc: "Base/BaseApp/Wiki/record-share URL to resolve",
			aliasFlags:  []string{"query"},
		},
		{
			shortcut:    "+title-resolve",
			definition:  BaseTitleResolve,
			primaryFlag: "title",
			primaryDesc: "Base title keyword",
			aliasFlags:  []string{"query", "url"},
		},
	} {
		t.Run(tc.shortcut, func(t *testing.T) {
			parent := &cobra.Command{Use: "base"}
			tc.definition.Mount(parent, &cmdutil.Factory{})
			cmd := parent.Commands()[0]
			primary := cmd.Flags().Lookup(tc.primaryFlag)
			primaryUsage := ""
			if primary != nil {
				primaryUsage = primary.Usage
			}
			if primary == nil || !strings.Contains(primaryUsage, tc.primaryDesc) {
				t.Fatalf("primary flag %q usage=%q", tc.primaryFlag, primaryUsage)
			}
			for _, aliasFlag := range tc.aliasFlags {
				alias := cmd.Flags().Lookup(aliasFlag)
				if alias != primary {
					t.Fatalf("Lookup(%q) = %#v, want canonical %#v", aliasFlag, alias, primary)
				}
			}
		})
	}
}

func TestBaseTitleResolve(t *testing.T) {
	t.Run("single result", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(titleResolveSearchStub([]interface{}{
			map[string]interface{}{
				"title_highlighted": "Sales <h>Pipeline</h>",
				"result_meta": map[string]interface{}{
					"doc_types":       "BITABLE",
					"token":           "bas123",
					"url":             "https://example.larkoffice.com/base/bas123",
					"owner_name":      "Alice",
					"update_time_iso": "2026-06-09T10:00:00+08:00",
				},
			},
		}))

		err := runShortcutWithAuthTypes(t, BaseTitleResolve, nil, []string{
			"+title-resolve", "--title", "Pipeline", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["title"] != "Sales Pipeline" || data["base_token"] != "bas123" || data["owner_name"] != "Alice" {
			t.Fatalf("unexpected output: %#v", data)
		}
	})

	t.Run("multiple results and filter non bitable", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(titleResolveSearchStub([]interface{}{
			map[string]interface{}{
				"title_highlighted": "Doc hit",
				"result_meta":       map[string]interface{}{"doc_types": "DOCX", "token": "docx123"},
			},
			map[string]interface{}{
				"title_highlighted": "Base <h>One</h>",
				"result_meta":       map[string]interface{}{"doc_types": "BITABLE", "token": "bas1", "url": "https://example/base/bas1"},
			},
			map[string]interface{}{
				"title_highlighted": "Base <h>Two</h>",
				"result_meta":       map[string]interface{}{"doc_types": "BITABLE", "token": "bas2", "url": "https://example/base/bas2"},
			},
		}))

		err := runShortcutWithAuthTypes(t, BaseTitleResolve, nil, []string{
			"+title-resolve", "--url", "Base", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		candidates, _ := data["candidates"].([]interface{})
		if len(candidates) != 2 {
			t.Fatalf("candidates=%#v, want 2", data["candidates"])
		}
	})

	t.Run("no results", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(titleResolveSearchStub(nil))
		err := runShortcutWithAuthTypes(t, BaseTitleResolve, nil, []string{
			"+title-resolve", "--title", "missing", "--as", "user",
		}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "No Base matched") {
			t.Fatalf("err=%v, want no result validation", err)
		}
	})

	t.Run("query too long", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseTitleResolve, nil, []string{
			"+title-resolve", "--title", "codex record share resolve 20260616152113", "--as", "user",
		}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "30 characters or fewer") {
			t.Fatalf("err=%v, want query length validation", err)
		}
	})
}

func titleResolveSearchStub(items []interface{}) *httpmock.Stub {
	if items == nil {
		items = []interface{}{}
	}
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/search/v2/doc_wiki/search",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"res_units": items,
			},
		},
	}
}

func fieldListStub(baseToken, tableID string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/" + baseToken + "/tables/" + tableID + "/fields",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"total": 2,
				"fields": []interface{}{
					map[string]interface{}{"field_id": "fld_name", "field_name": "Name", "type": "text"},
					map[string]interface{}{"field_id": "fld_status", "field_name": "Status", "type": "singleSelect"},
				},
			},
		},
	}
}

func recordBatchGetStub(baseToken, tableID, recordID string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/" + baseToken + "/tables/" + tableID + "/records/batch_get",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"record_id_list": []interface{}{recordID},
				"field_id_list":  []interface{}{"fld_name", "fld_status"},
				"fields":         []interface{}{"Name", "Status"},
				"data":           []interface{}{[]interface{}{"Alice", "Done"}},
			},
		},
	}
}
