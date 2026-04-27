// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
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

	listRT := newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, map[string]int{"offset": -1, "limit": 999})
	assertDryRunContains(t, dryRunTableList(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables", "offset=0", "limit=100")

	rt := newBaseTestRuntime(map[string]string{"base-token": "app_x", "table-id": "tbl_1", "name": "Orders"}, nil, nil)
	assertDryRunContains(t, dryRunTableGet(ctx, rt), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1")
	assertDryRunContains(t, dryRunTableCreate(ctx, rt), "POST /open-apis/base/v3/bases/app_x/tables")
	assertDryRunContains(t, dryRunTableUpdate(ctx, rt), "PATCH /open-apis/base/v3/bases/app_x/tables/tbl_1")
	assertDryRunContains(t, dryRunTableDelete(ctx, rt), "DELETE /open-apis/base/v3/bases/app_x/tables/tbl_1")
}

func TestDryRunFieldOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1"},
		nil,
		map[string]int{"offset": -2, "limit": 999},
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
		map[string]int{"offset": 3, "limit": 0},
	)
	assertDryRunContains(t, dryRunFieldGet(ctx, rt), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_1")
	assertDryRunContains(t, dryRunFieldCreate(ctx, rt), "POST /open-apis/base/v3/bases/app_x/tables/tbl_1/fields")
	assertDryRunContains(t, dryRunFieldUpdate(ctx, rt), "PUT /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_1")
	assertDryRunContains(t, dryRunFieldDelete(ctx, rt), "DELETE /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_1")
	assertDryRunContains(t, dryRunFieldSearchOptions(ctx, rt), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_1/options", "offset=3", "limit=30", "query=open")
}

func TestDryRunRecordOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "view-id": "viw_1"},
		map[string][]string{"field-id": {"Name", "Age"}},
		nil,
		map[string]int{"offset": -3, "limit": 500},
	)
	assertDryRunContains(t, dryRunRecordList(ctx, listRT), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/records", "offset=0", "limit=200", "view_id=viw_1", "field_id=Name", "field_id=Age")

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
	assertDryRunContains(t, dryRunRecordGet(ctx, rt), "GET /open-apis/base/v3/bases/app_x/tables/tbl_1/records/rec_1")
	assertDryRunContains(t, dryRunRecordUpsert(ctx, rt), "PATCH /open-apis/base/v3/bases/app_x/tables/tbl_1/records/rec_1")
	assertDryRunContains(t, dryRunRecordDelete(ctx, rt), "DELETE /open-apis/base/v3/bases/app_x/tables/tbl_1/records/rec_1")
	assertDryRunContains(t, dryRunRecordHistoryList(ctx, rt), "GET /open-apis/base/v3/bases/app_x/record_history", "max_version=11", "page_size=30", "record_id=rec_1", "table_id=tbl_1")

	uploadAttachmentRT := newBaseTestRuntime(
		map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_1",
			"record-id":  "rec_1",
			"field-id":   "fld_att",
			"file":       "/tmp/report.pdf",
			"name":       "report-final.pdf",
		},
		nil,
		nil,
	)
	assertDryRunContains(t,
		BaseRecordUploadAttachment.DryRun(ctx, uploadAttachmentRT),
		"GET /open-apis/base/v3/bases/app_x/tables/tbl_1/fields/fld_att",
		"GET /open-apis/base/v3/bases/app_x/tables/tbl_1/records/rec_1",
		"POST /open-apis/drive/v1/medias/upload_all",
		"bitable_file",
		"PATCH /open-apis/base/v3/bases/app_x/tables/tbl_1/records/rec_1",
		"report-final.pdf",
		`"mime_type":"\u003cdetected_mime_type\u003e"`,
		`"size":"\u003cfile_size\u003e"`,
		"deprecated_set_attachment",
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
			"page-size":    "50",
			"page-token":   "pt_1",
		},
		nil,
		nil,
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

func TestDryRunViewOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "table-id": "tbl_1", "view-id": "viw_1"},
		nil,
		map[string]int{"offset": -1, "limit": 500},
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
