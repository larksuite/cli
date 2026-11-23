// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsDBTableGet_DefaultJSONReturnsStructuredFields(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/tables/orders",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"name":        "orders",
				"description": "订单表",
				"columns": []interface{}{
					map[string]interface{}{
						"name": "id", "data_type": "int8",
						"is_primary_key": true, "is_unique": true,
						"is_allow_null": false, "default_value": "",
					},
				},
				"indexes": []interface{}{
					map[string]interface{}{"name": "orders_pkey", "type": "btree", "columns": []interface{}{"id"}, "definition": "..."},
				},
				"constraints": []interface{}{
					map[string]interface{}{"type": "primary_key", "name": "orders_pkey", "columns": []interface{}{"id"}},
				},
				"estimated_row_count": 1200,
				"size_bytes":          81920,
			},
		},
	})

	if err := runAppsShortcut(t, AppsDBTableGet,
		[]string{"+db-table-get", "--app-id", "app_x", "--table", "orders", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"name": "orders"`) {
		t.Fatalf("stdout missing schema name: %s", got)
	}
}

// --format pretty 是触发 DDL 模式的唯一开关。
// 用 --format json + --dry-run 走 JSON envelope 路径方便 parse，但 query 形态由代码内部
// 根据 rctx.Format 决定 —— 这里我们直接传 --format pretty + --dry-run，pretty 模式下 dry-run
// 输出是 plain text 列表，用 substring 校验 format=ddl 出现在 URL query 中。
func TestAppsDBTableGet_PrettyFormatSendsFormatDDLQuery(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBTableGet,
		[]string{"+db-table-get", "--app-id", "app_x", "--table", "orders", "--format", "pretty", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "/open-apis/spark/v1/apps/app_x/tables/orders") {
		t.Fatalf("missing URL in dry-run output:\n%s", got)
	}
	if !strings.Contains(got, "format=ddl") {
		t.Fatalf("--format=pretty should trigger ?format=ddl, got:\n%s", got)
	}
}

func TestAppsDBTableGet_NonPrettyFormatsOmitFormatQuery(t *testing.T) {
	// 默认 json / table / ndjson / csv 都走 schema 路径 —— CLI 不传 format query。
	for _, format := range []string{"json", "table", "ndjson", "csv"} {
		t.Run(format, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			args := []string{"+db-table-get", "--app-id", "app_x", "--table", "orders", "--format", format, "--dry-run", "--as", "user"}
			if err := runAppsShortcut(t, AppsDBTableGet, args, factory, stdout); err != nil {
				t.Fatalf("dry-run err=%v", err)
			}
			var env dryRunAPIEnvelope
			if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if _, ok := env.API[0].Params["format"]; ok {
				t.Fatalf("--format=%s should omit format query, got %v", format, env.API[0].Params)
			}
		})
	}
}

func TestAppsDBTableGet_PrettyOutputIsDDLTextOnly(t *testing.T) {
	// pretty 模式 stdout 直接打 ddl 字段文本，无 envelope / 表格包装。
	factory, stdout, reg := newAppsExecuteFactory(t)
	ddl := "CREATE TABLE orders (\n  id bigint NOT NULL,\n  PRIMARY KEY (id)\n);"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/tables/orders",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"ddl": ddl},
		},
	})

	if err := runAppsShortcut(t, AppsDBTableGet,
		[]string{"+db-table-get", "--app-id", "app_x", "--table", "orders", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "CREATE TABLE orders") {
		t.Fatalf("pretty output should contain raw DDL, got:\n%s", got)
	}
	if strings.Contains(got, `"data":`) || strings.Contains(got, `"ddl":`) {
		t.Fatalf("pretty output should not be JSON envelope, got:\n%s", got)
	}
}

func TestAppsDBTableGet_RequiresTable(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBTableGet,
		[]string{"+db-table-get", "--app-id", "app_x", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "table") {
		t.Fatalf("expected table required error, got %v", err)
	}
}
