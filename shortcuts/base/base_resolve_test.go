// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
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
		if data["table_id"] != "tbl123" || data["view_id"] != "vew123" || data["record_id"] != "rec123" {
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

	t.Run("field list enrichment failure still returns coordinates", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.larkoffice.com/base/bas123?table=tbl123", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		data := decodeBaseEnvelope(t, stdout)
		if data["base_token"] != "bas123" || data["table_id"] != "tbl123" {
			t.Fatalf("unexpected output: %#v", data)
		}
		hint, _ := data["hint"].(map[string]interface{})
		if hint["next_step"] != nextStepRecordList {
			t.Fatalf("unexpected hint: %#v", hint)
		}
		if _, ok := hint["fields"]; ok {
			t.Fatalf("fields should be omitted when enrichment fails: %#v", hint)
		}
	})
}

func TestBaseURLResolveWikiURL(t *testing.T) {
	t.Run("bitable", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/wiki/v2/spaces/get_node?token=wik123",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"node": map[string]interface{}{
						"obj_type":  "bitable",
						"obj_token": "bas123",
						"title":     "Demo Base",
					},
				},
			},
		})

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
		if !strings.Contains(nextStep, "+record-upsert --base-token bas123 --table-id tbl123 --record-id rec123") || recordData["fld_name"] != "Alice" || fields["total"] != float64(2) {
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
		if !strings.Contains(nextStep, "+record-upsert --base-token bas123 --table-id tbl123 --record-id rec123") {
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

func TestBaseResolveInputXOR(t *testing.T) {
	t.Run("url resolve", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseURLResolve, authTypes(), []string{
			"+url-resolve", "--url", "https://example.com/base/bas1", "--query", "https://example.com/base/bas2", "--as", "user",
		}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("err=%v, want xor validation", err)
		}
	})

	t.Run("title resolve", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcutWithAuthTypes(t, BaseTitleResolve, nil, []string{
			"+title-resolve", "--title", "Pipeline", "--query", "Sales", "--as", "user",
		}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("err=%v, want xor validation", err)
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
			primaryDesc: "Base/Wiki/record-share URL to resolve",
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
				if alias == nil || !alias.Hidden {
					t.Fatalf("alias flag %q should exist and be hidden: %#v", aliasFlag, alias)
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
