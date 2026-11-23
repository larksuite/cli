// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const dbDataExportURL = "/open-apis/spark/v1/apps/app_x/db/data_export"
const dbOrdersRecordsURL = "/open-apis/spark/v1/apps/app_x/tables/orders/records"

// TestAppsDBDataExport_RequiresTable 验证缺 --table 时报必填错误。
func TestAppsDBDataExport_RequiresTable(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	// 缺 --table → cobra required-flag, exit 1
	err := runAppsShortcut(t, AppsDBDataExport,
		[]string{"+db-data-export", "--app-id", "app_x", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected required-flag error for missing --table")
	}
}

// TestAppsDBDataExport_RejectsBadLimit 验证越界 --limit（0/-1/5001）均报 --limit 的 ValidationError。
func TestAppsDBDataExport_RejectsBadLimit(t *testing.T) {
	for _, lim := range []string{"0", "-1", "5001"} {
		factory, stdout, _ := newAppsExecuteFactory(t)
		err := runAppsShortcut(t, AppsDBDataExport,
			[]string{"+db-data-export", "--app-id", "app_x", "--table", "orders", "--limit", lim, "--as", "user"}, factory, stdout)
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("limit=%s err = %T %v, want *errs.ValidationError", lim, err, err)
		}
		if ve.Param != "--limit" {
			t.Fatalf("limit=%s Param = %q, want --limit", lim, ve.Param)
		}
	}
}

// TestAppsDBDataExport_RejectsBadOutputExtension 验证不支持的 --output 扩展名（.xml）报校验错误。
func TestAppsDBDataExport_RejectsBadOutputExtension(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBDataExport,
		[]string{"+db-data-export", "--app-id", "app_x", "--table", "orders", "--output", "dump.xml", "--as", "user"}, factory, stdout)
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected unsupported-format validation for .xml, got %v", err)
	}
}

// dry-run：format 跟随 --output 扩展名；缺省 csv。
// TestAppsDBDataExport_DryRunFormatFromOutput 验证 dry-run 的 format 参数跟随 --output 扩展名、缺省为 csv，并带 limit。
func TestAppsDBDataExport_DryRunFormatFromOutput(t *testing.T) {
	cases := []struct{ output, wantFmt string }{
		{"", "csv"}, {"orders.csv", "csv"}, {"orders.json", "json"}, {"dump.sql", "sql"},
	}
	for _, c := range cases {
		factory, stdout, _ := newAppsExecuteFactory(t)
		args := []string{"+db-data-export", "--app-id", "app_x", "--table", "orders", "--dry-run", "--as", "user"}
		if c.output != "" {
			args = append(args, "--output", c.output)
		}
		if err := runAppsShortcut(t, AppsDBDataExport, args, factory, stdout); err != nil {
			t.Fatalf("dry-run err=%v", err)
		}
		var env dryRunAPIEnvelope
		_ = json.Unmarshal([]byte(stdout.String()), &env)
		a := env.API[0]
		if a.Method != "GET" || a.URL != dbDataExportURL {
			t.Fatalf("dry-run = %s %s", a.Method, a.URL)
		}
		if a.Params["format"] != c.wantFmt || a.Params["table"] != "orders" {
			t.Errorf("output=%q params.format=%v want %q", c.output, a.Params["format"], c.wantFmt)
		}
		if _, ok := a.Params["limit"]; !ok {
			t.Errorf("dry-run missing limit param")
		}
	}
}

// 成功：先查 records 列表 total 计行，再把原始字节落盘。
// TestAppsDBDataExport_SuccessWritesFile 验证成功路径先查 records total 计行、再将导出原始字节落盘并输出 rows/format/table。
func TestAppsDBDataExport_SuccessWritesFile(t *testing.T) {
	dir := chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	// 第 1 步：records 列表 total=2（行数来源）。
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbOrdersRecordsURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"total": 2, "has_more": false, "items": "[]"}},
	})
	// 第 2 步：导出原始字节。
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     dbDataExportURL,
		RawBody: []byte("id,name\n1,a\n2,b\n"),
		Headers: http.Header{"Content-Type": []string{"text/csv"}},
	})
	if err := runAppsShortcut(t, AppsDBDataExport,
		[]string{"+db-data-export", "--app-id", "app_x", "--table", "orders", "--output", "orders.csv", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	b, err := os.ReadFile(dir + "/orders.csv")
	if err != nil || string(b) != "id,name\n1,a\n2,b\n" {
		t.Fatalf("output file wrong: %q err=%v", string(b), err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"rows": 2`) || !strings.Contains(got, `"format": "csv"`) || !strings.Contains(got, `"table": "orders"`) {
		t.Fatalf("output json missing fields:\n%s", got)
	}
}

// 行数取自 records total，且按 --limit 截顶（min(total, limit)）。
// TestAppsDBDataExport_RowsFromTotalCappedByLimit 验证行数取 records total 并按 --limit 截顶（total=10000、limit=100 → rows=100）。
func TestAppsDBDataExport_RowsFromTotalCappedByLimit(t *testing.T) {
	chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbOrdersRecordsURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"total": 10000, "has_more": true, "items": "[]"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbDataExportURL,
		RawBody: []byte("id\n1\n2\n3\n"), Headers: http.Header{"Content-Type": []string{"text/csv"}},
	})
	if err := runAppsShortcut(t, AppsDBDataExport,
		[]string{"+db-data-export", "--app-id", "app_x", "--table", "orders", "--output", "orders.csv", "--limit", "100", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), `"rows": 100`) {
		t.Fatalf("expected rows capped to limit 100 from total=10000:\n%s", stdout.String())
	}
}

// total 查询失败（records 列表报错）→ 回退按导出文件内容数行，不阻断导出。
// TestAppsDBDataExport_FallsBackToFileCountWhenTotalUnavailable 验证 records total 查询失败时回退按导出文件内容数行，不阻断落盘。
func TestAppsDBDataExport_FallsBackToFileCountWhenTotalUnavailable(t *testing.T) {
	dir := chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbOrdersRecordsURL,
		Body: map[string]interface{}{"code": 1254000, "msg": "records unavailable"},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbDataExportURL,
		RawBody: []byte("id,name\n1,a\n2,b\n3,c\n"), Headers: http.Header{"Content-Type": []string{"text/csv"}},
	})
	if err := runAppsShortcut(t, AppsDBDataExport,
		[]string{"+db-data-export", "--app-id", "app_x", "--table", "orders", "--output", "orders.csv", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("export should still succeed via fallback, got %v", err)
	}
	b, _ := os.ReadFile(dir + "/orders.csv")
	if string(b) != "id,name\n1,a\n2,b\n3,c\n" {
		t.Fatalf("file not written on fallback path: %q", string(b))
	}
	if !strings.Contains(stdout.String(), `"rows": 3`) {
		t.Fatalf("expected fallback file-count rows:3:\n%s", stdout.String())
	}
}

// 业务错误：网关回 JSON 信封 {code,msg}（非原始字节）→ typed error，不落盘。
// TestAppsDBDataExport_BusinessErrorEnvelope 验证响应为 JSON 错误信封（非原始字节）时返回 typed error 且不落盘。
func TestAppsDBDataExport_BusinessErrorEnvelope(t *testing.T) {
	chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     dbDataExportURL,
		RawBody: []byte(`{"code":1254043,"msg":"table not found"}`),
		Headers: http.Header{"Content-Type": []string{"application/json"}},
	})
	err := runAppsShortcut(t, AppsDBDataExport,
		[]string{"+db-data-export", "--app-id", "app_x", "--table", "nope", "--output", "nope.csv", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected business error to surface, got nil; stdout=%s", stdout.String())
	}
	if _, statErr := os.Stat("nope.csv"); statErr == nil {
		t.Fatalf("error path must not write the output file")
	}
}
