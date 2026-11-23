// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const dbChangelogURL = "/open-apis/spark/v1/apps/app_x/db/changelog_list"

// TestAppsDBChangelogList_RequiresAppID 验证空白 --app-id 报 --app-id 的 ValidationError。
func TestAppsDBChangelogList_RequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBChangelogList,
		[]string{"+db-changelog-list", "--app-id", "  ", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--app-id" {
		t.Fatalf("Param = %q, want --app-id", ve.Param)
	}
}

// TestAppsDBChangelogList_DryRunFiltersAndTimeNormalize 验证 dry-run 透传 env/table/change_id 过滤参数并将 since 归一化为 RFC3339 UTC。
func TestAppsDBChangelogList_DryRunFiltersAndTimeNormalize(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBChangelogList,
		[]string{"+db-changelog-list", "--app-id", "app_x", "--environment", "dev", "--table", "orders",
			"--change-id", "01J", "--since", "2026-01-01", "--page-size", "5", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	a := env.API[0]
	if a.Method != "GET" || a.URL != dbChangelogURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if a.Params["env"] != "dev" || a.Params["table"] != "orders" || a.Params["change_id"] != "01J" {
		t.Fatalf("params = %v", a.Params)
	}
	if s, _ := a.Params["since"].(string); !strings.HasSuffix(s, "Z") {
		t.Fatalf("since not normalized to RFC3339 UTC: %v", a.Params["since"])
	}
}

// TestAppsDBChangelogList_RejectsBadSince 验证不可解析的 --since 报 --since 的 ValidationError。
func TestAppsDBChangelogList_RejectsBadSince(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBChangelogList,
		[]string{"+db-changelog-list", "--app-id", "app_x", "--since", "notatime", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--since" {
		t.Fatalf("Param = %q, want --since", ve.Param)
	}
}

// TestAppsDBChangelogList_SuccessParsesOperator 验证成功响应中 operator JSON 串被解析为对象并输出变更字段。
func TestAppsDBChangelogList_SuccessParsesOperator(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbChangelogURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"has_more": false, "page_token": "",
			"items": []interface{}{map[string]interface{}{
				"change_id": "01J", "changed_at": "2026-04-15T10:30:00Z",
				"operator": `{"id":"7311","name":"alice"}`, "target_table": "orders",
				"change_type": "ALTER_TABLE", "summary": "add column", "statement": "ALTER TABLE orders ...",
			}},
		}},
	})
	if err := runAppsShortcut(t, AppsDBChangelogList,
		[]string{"+db-changelog-list", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{`"operator"`, `"name": "alice"`, `"id": "7311"`, `"change_type": "ALTER_TABLE"`, `"statement"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

// TestAppsDBChangelogList_ChangeIDNotFoundPretty 验证按 --change-id 查询无结果时 pretty 打印 not-found 提示。
func TestAppsDBChangelogList_ChangeIDNotFoundPretty(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: dbChangelogURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{}}},
	})
	if err := runAppsShortcut(t, AppsDBChangelogList,
		[]string{"+db-changelog-list", "--app-id", "app_x", "--change-id", "nope", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), "No DDL change with id=nope found.") {
		t.Fatalf("expected not-found message, got: %s", stdout.String())
	}
}

// TestParseOperator_Cases 验证 parseOperator 处理合法 JSON、空 name 回退 id、非 JSON 原样、空串返回 nil，以及 operatorName(nil) 为占位符。
func TestParseOperator_Cases(t *testing.T) {
	if op := parseOperator(`{"id":"1","name":"a"}`); op == nil || op.ID != "1" || op.Name != "a" {
		t.Fatalf("valid: %#v", op)
	}
	if op := parseOperator(`{"id":"1","name":""}`); op == nil || op.Name != "1" {
		t.Fatalf("name fallback to id: %#v", op)
	}
	if op := parseOperator("plain-user"); op == nil || op.ID != "plain-user" || op.Name != "plain-user" {
		t.Fatalf("non-json raw: %#v", op)
	}
	if op := parseOperator(""); op != nil {
		t.Fatalf("empty → nil, got %#v", op)
	}
	if operatorName(nil) != "—" {
		t.Fatalf("nil operatorName should be —")
	}
}

// TestSafeParseJSON_Cases 验证 safeParseJSON 合法 JSON 解析为对象、非法 JSON 原样返回字符串。
func TestSafeParseJSON_Cases(t *testing.T) {
	if v := safeParseJSON(`{"a":1}`); v == nil {
		t.Fatalf("valid json → object")
	}
	if v, ok := safeParseJSON("not json").(string); !ok || v != "not json" {
		t.Fatalf("invalid json → raw string, got %v", v)
	}
}
