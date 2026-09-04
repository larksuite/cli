// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func assertDryRunContains(t *testing.T, dr interface{ Format() string }, wants ...string) {
	t.Helper()
	out := dr.Format()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestDryRunTableOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, map[string]int{"offset": -1, "limit": 100})
	assertDryRunContains(t, dryRunTableList(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables", "offset=0", "limit=100")

	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "table-id": "tbl_1", "name": "Orders"}, nil, nil)
	assertDryRunContains(t, dryRunTableGet(ctx, rt), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1")

	// +table-create requires --fields, so the fieldless shape is unreachable
	// through the command surface; see table_create_test.go for that contract.
	tableCreateWithFieldsRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "name": "Orders", "fields": `[{"name":"Title","type":"text"}]`},
		nil,
		nil,
	)
	assertDryRunContains(t, dryRunTableCreate(ctx, tableCreateWithFieldsRT), "POST /open-apis/base/v3/bases/app_x/tables", `"fields":[{"name":"Title","type":"text"}]`)

	assertDryRunContains(t, dryRunTableUpdate(ctx, rt), "PATCH /open-apis/base/v3/bases/app_x/tables/tbl_1")
	assertDryRunContains(t, dryRunTableDelete(ctx, rt), "DELETE /open-apis/base/v3/bases/app_x/tables/tbl_1")
}

func TestDryRunTemplateCenterOps(t *testing.T) {
	ctx := context.Background()

	assertDryRunContains(t, dryRunTemplateCategories(ctx, newBaseTestRuntime(nil, nil, nil)), "GET /open-apis/base/v3/bases/templates/category")

	listRT := newBaseTestRuntime(map[string]string{"category-key": "office", "offset": "cursor_1"}, nil, map[string]int{"limit": 20})
	assertDryRunContains(t, dryRunTemplateList(ctx, listRT), "GET /open-apis/base/v3/bases/templates", "category_key=office", "limit=20", "offset=cursor_1")

	recommendedRT := newBaseTestRuntime(map[string]string{}, nil, map[string]int{"limit": 10})
	listOut := dryRunTemplateList(ctx, recommendedRT).Format()
	if strings.Contains(listOut, "category_key") {
		t.Fatalf("recommended template list must omit category_key when unset:\n%s", listOut)
	}

	searchRT := newBaseTestRuntime(map[string]string{"keyword": " AI ", "offset": "cursor_2"}, nil, map[string]int{"limit": 10})
	assertDryRunContains(t, dryRunTemplateSearch(ctx, searchRT), "GET /open-apis/base/v3/bases/templates/search", "keyword=AI", "limit=10", "offset=cursor_2")
}

func TestDryRunFieldExtensionOps(t *testing.T) {
	ctx := context.Background()

	baseFlags := map[string]string{"base-token": "app_x", "table-id": "tbl_x", "field-id": "fld_x"}
	assertDryRunContains(
		t,
		dryRunFieldExtensionGet(ctx, newBaseTestRuntime(baseFlags, nil, nil)),
		"GET /open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/field_extensions",
	)

	updateRT := newBaseTestRuntime(
		map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_x",
			"field-id":   "fld_x",
			"json":       `{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"text","text":"Summarize "},{"type":"field_ref","field":"Description"}]}}`,
		},
		nil,
		nil,
	)
	assertDryRunContains(
		t,
		dryRunFieldExtensionUpdate(ctx, updateRT),
		"PUT /open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/field_extensions",
		`"extension_id":"builtin_llm_completion"`,
		`"prompt":[{"type":"text","text":"Summarize "},{"type":"field_ref","field":"Description"}]`,
	)

	columnRT := newBaseTestRuntime(map[string]string{
		"base-token": "app_x",
		"table-id":   "tbl_x",
		"field-id":   "fld_x",
		"type":       "column",
		"view-id":    "vew_x",
	}, nil, nil)
	assertDryRunContains(
		t,
		dryRunFieldExtensionUpdateCells(ctx, columnRT),
		"POST /open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/field_extensions/update_cells",
		`"type":"column"`,
		`"view_id":"vew_x"`,
	)

	rowRT := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "table-id": "tbl_x", "field-id": "fld_x", "type": "row"},
		map[string][]string{"record-id": {"rec_1", "rec_2"}},
		nil,
		nil,
	)
	assertDryRunContains(
		t,
		dryRunFieldExtensionUpdateCells(ctx, rowRT),
		`"record_ids":["rec_1","rec_2"]`,
	)
}

func TestDryRunBaseBlockOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, nil)
	assertDryRunContains(t, dryRunBaseBlockList(ctx, listRT), "POST /open-apis/base/v3/bases/app_x/blocks/list")

	listFolderRT := newBaseTestRuntime(map[string]string{"base-token": "app_x", "parent-id": "bfl_1", "type": "docx"}, nil, nil)
	assertDryRunContains(t, dryRunBaseBlockList(ctx, listFolderRT), "POST /open-apis/base/v3/bases/app_x/blocks/list", `"parent_id":"bfl_1"`)

	createRT := newBaseTestRuntime(map[string]string{"base-token": "app_x", "type": "docx", "name": "Spec", "parent-id": "bfl_1"}, nil, nil)
	assertDryRunContains(t, dryRunBaseBlockCreate(ctx, createRT), "POST /open-apis/base/v3/bases/app_x/blocks", `"type":"docx"`, `"name":"Spec"`, `"parent_id":"bfl_1"`)

	moveRootRT := newBaseTestRuntime(map[string]string{"base-token": "app_x", "block-id": "blk_1"}, nil, nil)
	assertDryRunContains(t, dryRunBaseBlockMove(ctx, moveRootRT), "POST /open-apis/base/v3/bases/app_x/blocks/blk_1/move", `"parent_id":null`)

	moveAfterRT := newBaseTestRuntime(map[string]string{"base-token": "app_x", "block-id": "blk_1", "parent-id": "bfl_1", "after-id": "blk_0"}, nil, nil)
	assertDryRunContains(t, dryRunBaseBlockMove(ctx, moveAfterRT), "POST /open-apis/base/v3/bases/app_x/blocks/blk_1/move", `"parent_id":"bfl_1"`, `"after_id":"blk_0"`)

	renameRT := newBaseTestRuntime(map[string]string{"base-token": "app_x", "block-id": "blk_1", "name": "New name"}, nil, nil)
	assertDryRunContains(t, dryRunBaseBlockRename(ctx, renameRT), "POST /open-apis/base/v3/bases/app_x/blocks/blk_1/rename", `"name":"New name"`)
	assertDryRunContains(t, dryRunBaseBlockDelete(ctx, renameRT), "DELETE /open-apis/base/v3/bases/app_x/blocks/blk_1")
}

func TestDryRunFieldOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1"},
		nil,
		map[string]int{"offset": -2, "limit": 200},
	)
	assertDryRunContains(t, dryRunFieldList(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/fields", "offset=0", "limit=200")

	rt := newBaseTestRuntime(
		map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_1",
			"field-id":   "fld_1",
			"json":       `{"name":"Amount","type":"number"}`,
			"keyword":    " open ",
		},
		nil,
		map[string]int{"offset": 3, "limit": 30},
	)
	assertDryRunContains(t, dryRunFieldGet(ctx, rt), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_1")
	assertDryRunContains(t, dryRunFieldCreate(ctx, rt), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/fields")

	arrayRT := newBaseTestRuntime(
		map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_1",
			"json":       `[{"name":"A","type":"text"},{"name":"B","type":"text"}]`,
		},
		nil,
		nil,
	)
	assertDryRunContains(t, dryRunFieldCreate(ctx, arrayRT), `"name":"A"`, `"name":"B"`)

	assertDryRunContains(t, dryRunFieldUpdate(ctx, rt), "PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_1")
	assertDryRunContains(t, dryRunFieldDelete(ctx, rt), "DELETE /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_1")
	assertDryRunContains(t, dryRunFieldSearchOptions(ctx, rt), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_1/options", "offset=3", "limit=30", "query=open")

	autoNumberRT := newBaseTestRuntime(
		map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_1",
			"field-id":   "fld_1",
			"json":       `{"name":"编号","type":"auto_number","style":{"rules":[{"type":"text","text":"TASK-"},{"type":"created_time","date_format":"yyyyMM"},{"type":"text","text":"-"},{"type":"incremental_number","length":4}]}}`,
		},
		nil,
		nil,
	)
	autoNumberDR := dryRunFieldUpdate(ctx, autoNumberRT)
	assertDryRunContains(t, autoNumberDR, "PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_1", `"name":"编号"`, `"type":"auto_number"`, `"rules":[`, `"length":4`)
	if out := autoNumberDR.Format(); strings.Contains(out, "auto_serial") || strings.Contains(out, "reformat_existing_records") || strings.Contains(out, "/open-apis/bitable/v1/") {
		t.Fatalf("auto_number dry-run must stay on v3 field JSON, got:\n%s", out)
	}
}

func TestDryRunButtonRuleOps(t *testing.T) {
	ctx := context.Background()

	for _, fieldRef := range []string{"fld_1", "按钮"} {
		t.Run(fieldRef, func(t *testing.T) {
			rt := newBaseTestRuntime(
				map[string]string{
					"base-token":  "app_x",
					"table-id":    "tbl_1",
					"field-id":    fieldRef,
					"workflow-id": "wkf_1",
				},
				nil,
				nil,
			)
			resolvePath := baseV3Path("bases", "app_x", "tables", "tbl_1", "fields", fieldRef)
			buttonRulePath := "/open-apis/base/v3/bases/app_x/tables/tbl_1/fields/%3Cresolved_field_id%3E/button_rule"
			assertDryRunContains(t, BaseButtonRuleBind.DryRun(ctx, rt), "GET "+resolvePath, "PUT "+buttonRulePath, `"workflow_id":"wkf_1"`)
			assertDryRunContains(t, BaseButtonRuleGet.DryRun(ctx, rt), "GET "+resolvePath, "GET "+buttonRulePath)
			assertDryRunContains(t, BaseButtonRuleUnbind.DryRun(ctx, rt), "GET "+resolvePath, "PUT "+buttonRulePath, `"workflow_id":""`)

			dryRunJSON, err := json.Marshal(BaseButtonRuleBind.DryRun(ctx, rt))
			if err != nil {
				t.Fatalf("marshal dry-run: %v", err)
			}
			var envelope map[string]interface{}
			if err := json.Unmarshal(dryRunJSON, &envelope); err != nil {
				t.Fatalf("decode dry-run: %v", err)
			}
			if envelope["field_ref"] != fieldRef || envelope["resolved_field_id"] != "<resolved_field_id>" {
				t.Fatalf("dry-run field identity=%#v", envelope)
			}
			for _, want := range []string{`"desc":"Resolve --field-id as a field ID or name"`, `"desc":"Use the canonical field ID returned by step 1"`} {
				if !strings.Contains(string(dryRunJSON), want) {
					t.Fatalf("dry-run JSON missing %q: %s", want, dryRunJSON)
				}
			}
		})
	}
}

func TestDryRunRecordOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "view-id": "viw_1"},
		map[string][]string{"field-id": {"Name", "Age"}},
		nil,
		map[string]int{"offset": -3, "limit": 200},
	)
	assertDryRunContains(t, dryRunRecordList(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/records", "offset=0", "limit=200", "view_id=viw_1", "field_id=Name", "field_id=Age")

	listNDJSONRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "format": "ndjson", "output": "records.ndjson"},
		nil,
		map[string]int{"limit": 2000},
	)
	assertDryRunContains(
		t,
		dryRunRecordList(ctx, listNDJSONRT),
		"GET /open-apis/base/v3/bases/app_x/tables/tbl_1/records",
		"offset=0",
		"limit=500",
	)

	listFieldNamesAliasRT := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1"},
		map[string][]string{"field-names": {"Name", "Age"}},
		nil,
		map[string]int{"limit": 20},
	)
	assertDryRunContains(t, dryRunRecordList(ctx, listFieldNamesAliasRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/records", "limit=20", "field_id=Name", "field_id=Age")

	filteredListRT := newBaseTestRuntimeWithArrays(
		map[string]string{
			"base-token":  "app_x",
			"table-id":    "tbl_1",
			"filter-json": `{"logic":"and","conditions":[["Status","==","Todo"],["Score",">=",80]]}`,
			"sort-json":   `[{"field":"Due","desc":true}]`,
		},
		nil,
		nil,
		map[string]int{"limit": 20},
	)
	assertDryRunContains(
		t,
		dryRunRecordList(ctx, filteredListRT),
		"GET /open-apis/base/v3/bases/app_x/tables/tbl_1/records",
		"limit=20",
		"filter=%7B",
		"Status",
		"Todo",
		"sort=%5B",
		"Due",
	)

	commaFieldRT := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1"},
		map[string][]string{"field-id": {"A,B", "C"}},
		nil,
		map[string]int{"limit": 1},
	)
	assertDryRunContains(t, dryRunRecordList(ctx, commaFieldRT), "limit=1", "offset=0", "field_id=A%2CB", "field_id=C")

	searchRT := newBaseTestRuntime(
		map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_1",
			"json":       `{"view_id":"viw_1","keyword":"Created","search_fields":["Title","fld_owner"],"select_fields":["Title","fld_owner"],"offset":-1,"limit":500}`,
		},
		nil, nil,
	)
	assertDryRunContains(
		t,
		dryRunRecordSearch(ctx, searchRT),
		"POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/search",
		`"view_id":"viw_1"`,
		`"keyword":"Created"`,
		`"search_fields":["Title","fld_owner"]`,
		`"select_fields":["Title","fld_owner"]`,
		`"offset":-1`,
		`"limit":500`,
	)

	searchNDJSONRT := newBaseTestRuntime(
		map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_1",
			"format":     "ndjson",
			"output":     "search.ndjson",
			"json":       `{"keyword":"Created","search_fields":["Title"],"limit":2000}`,
		},
		nil,
		nil,
	)
	assertDryRunContains(
		t,
		dryRunRecordSearch(ctx, searchNDJSONRT),
		"POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/search",
		`"limit":500`,
	)

	searchFlagRT := newBaseTestRuntimeWithArrays(
		map[string]string{
			"base-token":  "app_x",
			"table-id":    "tbl_1",
			"keyword":     "Alice",
			"view-id":     "viw_1",
			"filter-json": `{"logic":"and","conditions":[["Status","!=","Done"]]}`,
			"sort-json":   `[{"field":"Updated At","desc":true}]`,
		},
		map[string][]string{
			"search-field": {"Name"},
			"field-id":     {"Name", "Status"},
		},
		nil,
		map[string]int{"limit": 20},
	)
	assertDryRunContains(
		t,
		dryRunRecordSearch(ctx, searchFlagRT),
		"POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/search",
		`"keyword":"Alice"`,
		`"search_fields":["Name"]`,
		`"select_fields":["Name","Status"]`,
		`"filter":{"conditions":[["Status","!=","Done"]],"logic":"and"}`,
		`"sort":[{"desc":true,"field":"Updated At"}]`,
	)

	upsertCreateRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "json": `{"Name":"A"}`},
		nil, nil,
	)
	assertDryRunContains(t, dryRunRecordUpsert(ctx, upsertCreateRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records")
	assertDryRunContains(t, dryRunRecordBatchCreate(ctx, upsertCreateRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/batch_create")
	assertDryRunContains(t, dryRunRecordBatchUpdate(ctx, upsertCreateRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/batch_update")

	rt := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "record-id": "rec_1", "json": `{"Name":"B"}`},
		nil,
		map[string]int{"max-version": 11, "page-size": 30},
	)
	assertDryRunContains(t, dryRunRecordUpsert(ctx, rt), "PATCH /open-apis/base/v3/bases/app_x/tables/tbl_1/records/rec_1")
	assertDryRunContains(t, dryRunRecordHistoryList(ctx, rt), "GET /open-apis/base/v3/bases/app_x/record_history", "max_version=11", "page_size=30", "record_id=rec_1", "table_id=tbl_1")

	getSingleRT := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1"},
		map[string][]string{"record-id": {"rec_1"}},
		nil,
		nil,
	)
	assertDryRunContains(t, dryRunRecordGet(ctx, getSingleRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/batch_get", `"record_id_list":["rec_1"]`)
	assertDryRunContains(t, dryRunRecordDelete(ctx, getSingleRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/batch_delete", `"record_id_list":["rec_1"]`)

	getSingleFieldsRT := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1"},
		map[string][]string{"record-id": {"rec_1"}, "field-id": {"Name", "Age"}},
		nil,
		nil,
	)
	assertDryRunContains(t, dryRunRecordGet(ctx, getSingleFieldsRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/batch_get", `"record_id_list":["rec_1"]`, `"select_fields":["Name","Age"]`)

	getBatchRT := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1"},
		map[string][]string{"record-id": {"rec_2", "rec_1"}, "field-id": {"Name", "Age"}},
		nil,
		nil,
	)
	assertDryRunContains(t, dryRunRecordGet(ctx, getBatchRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/batch_get", `"record_id_list":["rec_2","rec_1"]`, `"select_fields":["Name","Age"]`)
	assertDryRunContains(t, dryRunRecordDelete(ctx, getBatchRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/batch_delete", `"record_id_list":["rec_2","rec_1"]`)

	getJSONRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "json": `{"record_id_list":["rec_3"],"select_fields":["Status"]}`},
		nil,
		nil,
	)
	assertDryRunContains(t, dryRunRecordGet(ctx, getJSONRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/batch_get", `"record_id_list":["rec_3"]`, `"select_fields":["Status"]`)
	assertDryRunContains(t, dryRunRecordDelete(ctx, getJSONRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/records/batch_delete", `"record_id_list":["rec_3"]`)

	uploadAttachmentRT := newBaseTestRuntimeWithArrays(
		map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_1",
			"record-id":  "rec_1",
			"field-id":   "fld_att",
		},
		map[string][]string{"file": {"/tmp/report.pdf"}},
		nil,
		nil,
	)
	assertDryRunContains(t,
		BaseRecordUploadAttachment.DryRun(ctx, uploadAttachmentRT),
		"GET /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_att",
		"POST /open-apis/drive/v1/medias/upload_all",
		"bitable_file",
		"POST /open-apis/base/v3/bases/app_x/tables/tbl_1/append_attachments",
		"report.pdf",
		`"image_width":"\u003cimage_width_if_image\u003e"`,
		`"image_height":"\u003cimage_height_if_image\u003e"`,
	)
}

func TestDryRunBaseOps(t *testing.T) {
	ctx := context.Background()

	getRT := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, nil)
	assertDryRunContains(t, dryRunBaseGet(ctx, getRT), "GET /open-apis/base/v3/bases/app_x")

	copyRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "name": "Copied", "folder-token": "fld_x", "time-zone": "Asia/Shanghai"},
		map[string]bool{"without-content": true},
		nil,
	)
	assertDryRunContains(t, dryRunBaseCopy(ctx, copyRT), "POST /open-apis/base/v3/bases/app_x/copy")

	createRT := newBaseTestRuntime(
		map[string]string{"name": "New Base", "folder-token": "fld_y", "time-zone": "Asia/Shanghai"},
		nil,
		nil,
	)
	assertDryRunContains(t, dryRunBaseCreate(ctx, createRT), "POST /open-apis/base/v3/bases")

	createWithFieldsRT := newBaseTestRuntime(
		map[string]string{"name": "New Base", "table-name": "Tasks", "fields": `[{"name":"Title","type":"text"},{"name":"Status","type":"text"}]`},
		nil,
		nil,
	)
	assertDryRunContains(
		t,
		dryRunBaseCreate(ctx, createWithFieldsRT),
		"POST /open-apis/base/v3/bases",
		"GET /open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables?limit=100&offset=0",
		"POST /open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables",
		`"name":"Tasks"`,
		`"fields":[{"name":"Title","type":"text"},{"name":"Status","type":"text"}]`,
		"DELETE /open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables/%3Cdefault_table_id%3E",
	)

	createWithFieldsDefaultNameRT := newBaseTestRuntime(
		map[string]string{"name": "New Base", "fields": `[{"name":"Title","type":"text"}]`},
		nil,
		nil,
	)
	assertDryRunContains(
		t,
		dryRunBaseCreate(ctx, createWithFieldsDefaultNameRT),
		"POST /open-apis/base/v3/bases",
		"POST /open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables",
		`"name":"Table 1"`,
		`"fields":[{"name":"Title","type":"text"}]`,
		"DELETE /open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables/%3Cdefault_table_id%3E",
	)

	createWithTableNameRT := newBaseTestRuntime(
		map[string]string{"name": "New Base", "table-name": "Tasks"},
		nil,
		nil,
	)
	assertDryRunContains(
		t,
		dryRunBaseCreate(ctx, createWithTableNameRT),
		"POST /open-apis/base/v3/bases",
		"GET /open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables?limit=100&offset=0",
		"PATCH /open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables/%3Cdefault_table_id%3E",
		`"name":"Tasks"`,
	)
}

func TestDryRunDashboardOps(t *testing.T) {
	ctx := context.Background()

	rt := newBaseTestRuntime(
		map[string]string{
			"base-token":   "app_x",
			"dashboard-id": "dash_1",
			"block-id":     "blk_1",
			"name":         "Main",
			"theme-style":  "light",
			"type":         "bar",
			"data-config":  `{"table_name":"orders"}`,
			"user-id-type": "open_id",
			"page-token":   "pt_1",
		},
		nil,
		map[string]int{"page-size": 50},
	)

	assertDryRunContains(t, dryRunDashboardList(ctx, rt), "GET /open-apis/base/v3/bases/app_x/dashboards", "page_size=50", "page_token=pt_1")
	assertDryRunContains(t, dryRunDashboardGet(ctx, rt), "GET /open-apis/base/v3/bases/app_x/dashboards/dash_1")
	assertDryRunContains(t, dryRunDashboardCreate(ctx, rt), "POST /open-apis/base/v3/bases/app_x/dashboards")
	assertDryRunContains(t, dryRunDashboardUpdate(ctx, rt), "PATCH /open-apis/base/v3/bases/app_x/dashboards/dash_1")
	assertDryRunContains(t, dryRunDashboardDelete(ctx, rt), "DELETE /open-apis/base/v3/bases/app_x/dashboards/dash_1")

	assertDryRunContains(t, dryRunDashboardBlockList(ctx, rt), "GET /open-apis/base/v3/bases/app_x/dashboards/dash_1/blocks", "page_size=50", "page_token=pt_1")
	assertDryRunContains(t, dryRunDashboardBlockGet(ctx, rt), "GET /open-apis/base/v3/bases/app_x/dashboards/dash_1/blocks/blk_1", "user_id_type=open_id")
	assertDryRunContains(t, dryRunDashboardBlockCreate(ctx, rt), "POST /open-apis/base/v3/bases/app_x/dashboards/dash_1/blocks", "user_id_type=open_id")
	assertDryRunContains(t, dryRunDashboardBlockUpdate(ctx, rt), "PATCH /open-apis/base/v3/bases/app_x/dashboards/dash_1/blocks/blk_1", "user_id_type=open_id")
	assertDryRunContains(t, dryRunDashboardBlockDelete(ctx, rt), "DELETE /open-apis/base/v3/bases/app_x/dashboards/dash_1/blocks/blk_1")
}

// TestDryRunDashboardBlockPositionAndNumberFormat asserts the shared body
// builder surfaces the optional top-level position and data_config.number_format
// in the dry-run request body, and that the update preview never carries type
// (which the API rejects). Preview-vs-executed body equality is proven end to
// end by TestBaseDashboardBlockDryRunMatchesExecuteBody.
func TestDryRunDashboardBlockPositionAndNumberFormat(t *testing.T) {
	ctx := context.Background()

	rt := newBaseTestRuntime(
		map[string]string{
			"base-token":   "app_x",
			"dashboard-id": "dash_1",
			"block-id":     "blk_1",
			"name":         "Revenue",
			"type":         "statistics",
			"data-config":  `{"table_name":"Orders","count_all":true,"number_format":{"formatName":"dollar_rounded","precision":2}}`,
			"position":     `{"x":0,"y":0,"w":6,"h":4}`,
		},
		nil,
		nil,
	)

	assertDryRunContains(t, dryRunDashboardBlockCreate(ctx, rt),
		"POST /open-apis/base/v3/bases/app_x/dashboards/dash_1/blocks",
		`"position"`, `"w":6`, `"number_format"`, `"dollar_rounded"`)
	assertDryRunContains(t, dryRunDashboardBlockUpdate(ctx, rt),
		"PATCH /open-apis/base/v3/bases/app_x/dashboards/dash_1/blocks/blk_1",
		`"position"`, `"w":6`)

	pc := newParseCtx(rt)
	updateBody, err := buildDashboardBlockBody(pc, rt, false)
	if err != nil {
		t.Fatalf("update body build err=%v", err)
	}
	if _, hasType := updateBody["type"]; hasType {
		t.Fatalf("update body must not carry type: %v", updateBody)
	}
}

func TestDryRunViewOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "view-id": "viw_1"},
		nil,
		map[string]int{"offset": -1, "limit": 200},
	)
	assertDryRunContains(t, dryRunViewList(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views", "offset=0", "limit=200")
	assertDryRunContains(t, dryRunViewGet(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1")
	assertDryRunContains(t, dryRunViewDelete(ctx, listRT), "DELETE /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1")

	createValidRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "json": `[{"name":"Main"}]`},
		nil, nil,
	)
	assertDryRunContains(t, dryRunViewCreate(ctx, createValidRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/views")

	createInvalidRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "json": `{`},
		nil, nil,
	)
	assertDryRunContains(t, dryRunViewCreate(ctx, createInvalidRT), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/views")

	setJSONObjectRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "view-id": "viw_1", "json": `{"enabled":true}`, "name": "New View"},
		nil, nil,
	)
	assertDryRunContains(t, dryRunViewSetFilter(ctx, setJSONObjectRT), "PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/filter")
	assertDryRunContains(t, dryRunViewSetTimebar(ctx, setJSONObjectRT), "PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/timebar")
	assertDryRunContains(t, dryRunViewSetCard(ctx, setJSONObjectRT), "PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/card")
	assertDryRunContains(t, dryRunViewRename(ctx, setJSONObjectRT), "PATCH /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1")

	setWrappedRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "view-id": "viw_1", "json": `[{"field":"fld_status"}]`},
		nil, nil,
	)
	assertDryRunContains(t, dryRunViewSetGroup(ctx, setWrappedRT), "PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/group")
	assertDryRunContains(t, dryRunViewSetSort(ctx, setWrappedRT), "PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/sort")

	setWrappedInvalidRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "view-id": "viw_1", "json": `{`},
		nil, nil,
	)
	assertDryRunContains(t, dryRunViewSetWrapped(setWrappedInvalidRT, "group", "group_config"), "PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/group")

	assertDryRunContains(t, dryRunViewGetFilter(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/filter")
	assertDryRunContains(t, dryRunViewGetVisibleFields(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/visible_fields")
	assertDryRunContains(t, dryRunViewGetGroup(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/group")
	assertDryRunContains(t, dryRunViewGetSort(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/sort")
	assertDryRunContains(t, dryRunViewGetTimebar(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/timebar")
	assertDryRunContains(t, dryRunViewGetCard(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/card")

	assertDryRunContains(t, dryRunViewGetProperty(listRT, "a/b"), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views/viw_1/a%2Fb")
}

func TestDryRunFormVisibleFieldsPreservesCompleteTargetState(t *testing.T) {
	ctx := context.Background()
	formRT := newBaseTestRuntime(
		map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_1",
			"view-id":    "vew_form",
			"json":       `{"visible_fields":["fld_c","fld_a"]}`,
		},
		nil,
		nil,
	)

	assertDryRunContains(
		t,
		dryRunViewGetVisibleFields(ctx, formRT),
		"GET /open-apis/base/v3/bases/app_x/tables/tbl_1/views/vew_form/visible_fields",
	)
	assertDryRunContains(
		t,
		dryRunViewSetVisibleFields(ctx, formRT),
		"PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/views/vew_form/visible_fields",
		`"visible_fields":["fld_c","fld_a"]`,
	)
}
