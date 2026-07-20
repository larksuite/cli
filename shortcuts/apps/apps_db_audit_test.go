// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const (
	dbAuditStatusURL = "/open-apis/spark/v1/apps/app_x/db/audit_status"
	dbAuditSetURL    = "/open-apis/spark/v1/apps/app_x/db/audit_set"
	dbAuditListURL   = "/open-apis/spark/v1/apps/app_x/db/audit_list"
	dbTablesListURL  = "/open-apis/spark/v1/apps/app_x/tables"
)

// ── audit-status ──

// TestAppsDBAuditStatus_SingleTableObjectWithPlaceholder 验证单表查询无记录时返回 enabled:false 的占位对象（非数组）。
func TestAppsDBAuditStatus_SingleTableObjectWithPlaceholder(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbAuditStatusURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{}}},
	})
	if err := runAppsShortcut(t, AppsDBAuditStatus,
		[]string{"+db-audit-status", "--app-id", "app_x", "--table", "orders", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	// 单表无记录 → 占位对象 enabled:false（不是数组）。
	var env struct {
		Data struct {
			Table   string `json:"table"`
			Enabled bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if env.Data.Table != "orders" || env.Data.Enabled {
		t.Fatalf("expected placeholder {orders,false}, got %+v", env.Data)
	}
}

// TestAppsDBAuditStatus_MultiTablePrettyTable 验证多表 pretty 输出含 enabled/yes/no 列与 retention 值。
func TestAppsDBAuditStatus_MultiTablePrettyTable(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbAuditStatusURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{
			map[string]interface{}{"table": "orders", "enabled": true, "enabled_at": "2026-04-15T10:30:00Z", "retention": "30d"},
			map[string]interface{}{"table": "users", "enabled": false},
		}}},
	})
	if err := runAppsShortcut(t, AppsDBAuditStatus,
		[]string{"+db-audit-status", "--app-id", "app_x", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "enabled") || !strings.Contains(got, "yes") || !strings.Contains(got, "no") || !strings.Contains(got, "30d") {
		t.Fatalf("pretty table malformed:\n%s", got)
	}
}

// ── audit-enable / disable ──

// TestAppsDBAuditEnable_RequiresTableAndValidRetention 验证缺 --table 报必填错、非法 --retention 报 ValidationError。
func TestAppsDBAuditEnable_RequiresTableAndValidRetention(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	// 缺 --table → cobra required, exit 1
	if err := runAppsShortcut(t, AppsDBAuditEnable,
		[]string{"+db-audit-enable", "--app-id", "app_x", "--as", "user"}, factory, stdout); err == nil {
		t.Fatalf("expected required --table error")
	}
	// 非法 retention → enum 校验 (validation)
	factory2, stdout2, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBAuditEnable,
		[]string{"+db-audit-enable", "--app-id", "app_x", "--table", "orders", "--retention", "99d", "--as", "user"}, factory2, stdout2)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--retention" {
		t.Fatalf("Param = %q, want --retention", ve.Param)
	}
}

// TestAppsDBAuditEnable_DryRunAndSuccess 验证 dry-run 发出 enabled:true+retention 的 POST，成功时打印 pretty 确认行。
func TestAppsDBAuditEnable_DryRunAndSuccess(t *testing.T) {
	// dry-run body {table, enabled:true, retention}
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBAuditEnable,
		[]string{"+db-audit-enable", "--app-id", "app_x", "--table", "orders", "--retention", "30d", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		Data struct {
			API []struct {
				Method string                 `json:"method"`
				URL    string                 `json:"url"`
				Body   map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.Data.API[0]
	if a.Method != "POST" || a.URL != dbAuditSetURL || a.Body["enabled"] != true || a.Body["retention"] != "30d" || a.Body["table"] != "orders" {
		t.Fatalf("dry-run = %s %s body=%v", a.Method, a.URL, a.Body)
	}

	// success
	factory2, stdout2, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbAuditSetURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"status": map[string]interface{}{"table": "orders", "enabled": true, "retention": "30d"}}},
	})
	if err := runAppsShortcut(t, AppsDBAuditEnable,
		[]string{"+db-audit-enable", "--app-id", "app_x", "--table", "orders", "--retention", "30d", "--format", "pretty", "--as", "user"}, factory2, stdout2); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout2.String(), "✓ Audit enabled for table 'orders' (retention: 30d)") {
		t.Fatalf("pretty: %s", stdout2.String())
	}
}

// TestAppsDBAuditDisable_DryRunAndSuccess 验证 dry-run 发出 enabled:false 的 POST，成功时打印 pretty 确认行。
func TestAppsDBAuditDisable_DryRunAndSuccess(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBAuditDisable,
		[]string{"+db-audit-disable", "--app-id", "app_x", "--table", "orders", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		Data struct {
			API []struct {
				Body map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	if env.Data.API[0].Body["enabled"] != false || env.Data.API[0].Body["table"] != "orders" {
		t.Fatalf("dry-run body=%v (want enabled:false)", env.Data.API[0].Body)
	}

	factory2, stdout2, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbAuditSetURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"status": map[string]interface{}{"table": "orders", "enabled": false}}},
	})
	if err := runAppsShortcut(t, AppsDBAuditDisable,
		[]string{"+db-audit-disable", "--app-id", "app_x", "--table", "orders", "--format", "pretty", "--as", "user"}, factory2, stdout2); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout2.String(), "✓ Audit disabled for table 'orders'") {
		t.Fatalf("pretty: %s", stdout2.String())
	}
}

// ── audit-list ──

// TestAppsDBAuditList_RequiresTable 验证缺 --table 时报必填错误。
func TestAppsDBAuditList_RequiresTable(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBAuditList,
		[]string{"+db-audit-list", "--app-id", "app_x", "--as", "user"}, factory, stdout); err == nil {
		t.Fatalf("expected required --table error")
	}
}

// TestAppsDBAuditList_DryRunJoinsTables 验证 dry-run 将多个 --table 合并为 tables=orders,users 且归一化 since。
func TestAppsDBAuditList_DryRunJoinsTables(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBAuditList,
		[]string{"+db-audit-list", "--app-id", "app_x", "--table", "orders", "--table", "users", "--since", "7d", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		Data struct {
			API []struct {
				Method string                 `json:"method"`
				URL    string                 `json:"url"`
				Params map[string]interface{} `json:"params"`
			} `json:"api"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.Data.API[0]
	if a.Method != "GET" || a.URL != dbAuditListURL || a.Params["tables"] != "orders,users" {
		t.Fatalf("dry-run = %s %s tables=%v", a.Method, a.URL, a.Params["tables"])
	}
	if s, _ := a.Params["since"].(string); !strings.HasSuffix(s, "Z") {
		t.Fatalf("since not normalized: %v", a.Params["since"])
	}
}

// 单表查询：不预过滤、直接打 audit_list（后端就 not-found/not-enabled 报错），无 skipped。
// TestAppsDBAuditList_SingleTableNoPreflight 验证单表查询不预过滤、operator/before/after 还原为对象、无 skipped。
func TestAppsDBAuditList_SingleTableNoPreflight(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbAuditListURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"has_more": false, "page_token": "",
			"items": []interface{}{map[string]interface{}{
				"event_id": "01525", "event_time": "2026-04-16T10:30:00Z", "target_table": "users",
				"type": "UPDATE", "operator": `{"id":"7311","name":"alice"}`, "summary": "UPDATE 1 field",
				"before": `{"amount":100}`, "after": `{"amount":999}`,
			}},
		}},
	})
	if err := runAppsShortcut(t, AppsDBAuditList,
		[]string{"+db-audit-list", "--app-id", "app_x", "--table", "users", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	// operator → 对象；before/after → 还原成对象（非字符串）。
	for _, want := range []string{`"name": "alice"`, `"before"`, `"amount": 100`, `"after"`, `"amount": 999`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"skipped"`) {
		t.Errorf("single-table query must not emit skipped:\n%s", got)
	}
	if strings.Contains(got, `"before": "{`) {
		t.Errorf("before should be an object, not a JSON string:\n%s", got)
	}
}

// TestAppsDBAuditList_SingleTableEmptyPretty 验证单表无事件时不报错、pretty 打印 "No audit events found." 且无 Skipped。
func TestAppsDBAuditList_SingleTableEmptyPretty(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbAuditListURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{}}},
	})
	if err := runAppsShortcut(t, AppsDBAuditList,
		[]string{"+db-audit-list", "--app-id", "app_x", "--table", "orders", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("empty audit list should NOT error (ok read), got %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "No audit events found.") || strings.Contains(got, "Skipped") {
		t.Fatalf("expected empty, no skipped for single table:\n%s", got)
	}
}

// 多表查询：CLI 用 schema（存在性）+ status（审计开关）预过滤，只把有效表传给 audit_list，
// 不存在 / 未开启审计的表进 skipped。
// TestAppsDBAuditList_MultiTablePreflightFilters 验证多表查询用 schema+status 预过滤，仅传有效表，不存在/未开审计的表进 skipped。
func TestAppsDBAuditList_MultiTablePreflightFilters(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	// schema：orders/users/carts 存在，ghost 不存在。
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbTablesListURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"has_more": false, "items": []interface{}{
			map[string]interface{}{"name": "orders"}, map[string]interface{}{"name": "users"}, map[string]interface{}{"name": "carts"},
		}}},
	})
	// status：orders/users 开启审计，carts 未开启。
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbAuditStatusURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{
			map[string]interface{}{"table": "orders", "enabled": true}, map[string]interface{}{"table": "users", "enabled": true},
			map[string]interface{}{"table": "carts", "enabled": false},
		}}},
	})
	// audit_list 只应被传入有效表 orders,users。
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbAuditListURL,
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("tables"); got != "orders,users" {
				t.Errorf("audit_list tables = %q, want orders,users (filtered)", got)
			}
		},
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"has_more": false, "items": []interface{}{
			map[string]interface{}{"event_id": "e1", "event_time": "2026-04-16T10:30:00Z", "target_table": "orders", "type": "INSERT", "summary": "INSERT"},
		}}},
	})
	if err := runAppsShortcut(t, AppsDBAuditList,
		[]string{"+db-audit-list", "--app-id", "app_x", "--table", "orders", "--table", "users", "--table", "carts", "--table", "ghost", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	// skipped：carts(audit not enabled) + ghost(table not found)，结构化 {table,reason}。
	for _, want := range []string{`"skipped"`, `"table": "carts"`, `"reason": "audit not enabled"`, `"table": "ghost"`, `"reason": "table not found"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

// 多表查询且全部被过滤掉 → 不调 audit_list，直接空 + skipped 提示。
// TestAppsDBAuditList_MultiTableAllFilteredSkipsQuery 验证多表全部被过滤时跳过 audit_list 调用，直接输出空结果加 Skipped 提示。
func TestAppsDBAuditList_MultiTableAllFilteredSkipsQuery(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbTablesListURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"has_more": false, "items": []interface{}{
			map[string]interface{}{"name": "orders"},
		}}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbAuditStatusURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{}}},
	})
	// 不注册 audit_list：若被调用会命中未注册请求而报错。
	if err := runAppsShortcut(t, AppsDBAuditList,
		[]string{"+db-audit-list", "--app-id", "app_x", "--table", "ghost1", "--table", "ghost2", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("all-filtered should still succeed (empty), got %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "No audit events found.") || !strings.Contains(got, "Skipped 2 of 2 tables") {
		t.Fatalf("expected empty + 'Skipped 2 of 2 tables':\n%s", got)
	}
}
