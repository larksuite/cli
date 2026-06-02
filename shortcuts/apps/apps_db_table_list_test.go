// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

// TestAppsDBTableList_BusinessErrorSurfacedAsTypedEnvelope 验证 server 业务错误
// （code != 0，如单环境 app 查 env=dev 返 "Invalid DB Branch"）被 CLI 透出成
// typed api_error envelope —— 用 BOE 实测的错误码 / 文案做输入。
func TestAppsDBTableList_BusinessErrorSurfacedAsTypedEnvelope(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/tables",
		Body: map[string]interface{}{
			"code": 500002511,
			"msg":  "k_dl_1600000：Invalid DB Branch：dev",
		},
	})

	err := runAppsShortcut(t, AppsDBTableList,
		[]string{"+db-table-list", "--app-id", "app_x", "--env", "dev", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("expected business error to surface, got nil; stdout=%s", stdout.String())
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected ExitError with detail, got %v", err)
	}
	if exitErr.Detail.Type != "api_error" {
		t.Fatalf("error.type = %q, want api_error", exitErr.Detail.Type)
	}
	if exitErr.Detail.Code != 500002511 {
		t.Fatalf("error.code = %d, want 500002511", exitErr.Detail.Code)
	}
	if !strings.Contains(exitErr.Detail.Message, "Invalid DB Branch") {
		t.Fatalf("error.message missing 'Invalid DB Branch': %q", exitErr.Detail.Message)
	}
}

func TestAppsDBTableList_SuccessReturnsItemsWithStats(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/tables",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":   false,
				"page_token": "",
				"items": []interface{}{
					map[string]interface{}{
						"name":                "orders",
						"description":         "订单表",
						"columns":             []interface{}{map[string]interface{}{"name": "id"}, map[string]interface{}{"name": "user_id"}},
						"estimated_row_count": 1200,
						"size_bytes":          81920,
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsDBTableList,
		[]string{"+db-table-list", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"name": "orders"`) {
		t.Fatalf("stdout missing table name: %s", got)
	}
	if !strings.Contains(got, `"estimated_row_count": 1200`) {
		t.Fatalf("stdout missing estimated_row_count: %s", got)
	}
}

// pretty 5 列 + 列名 (size / columns，不是 size_bytes / column_count) + size 友好格式（KB） +
// 空 description 用 "—" 占位。
func TestAppsDBTableList_PrettyRendersFiveColumnsHumanReadable(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/tables",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"name":                "orders",
						"description":         "Order entries",
						"columns":             []interface{}{map[string]interface{}{"name": "id"}, map[string]interface{}{"name": "user_id"}},
						"estimated_row_count": 1200,
						"size_bytes":          81920, // 80 KB
					},
					map[string]interface{}{
						"name":                "customers",
						"description":         "",
						"columns":             []interface{}{map[string]interface{}{"name": "id"}},
						"estimated_row_count": 350,
						"size_bytes":          24576, // 24 KB
					},
				},
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBTableList,
		[]string{"+db-table-list", "--app-id", "app_x", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	// Header 行 5 列命名。
	wantHeader := "name       description    estimated_row_count  size   columns"
	// rows
	wantOrders := "orders     Order entries  1200                 80 KB  2"
	wantCustomers := "customers  —              350                  24 KB  1"
	for _, want := range []string{wantHeader, wantOrders, wantCustomers} {
		if !strings.Contains(got, want) {
			t.Errorf("missing line %q\nactual output:\n%s", want, got)
		}
	}
	// 禁止出现旧列名 / 原始字节。
	for _, banned := range []string{"size_bytes", "column_count", "81920", "24576"} {
		if strings.Contains(got, banned) {
			t.Errorf("pretty output contains %q (must be human-formatted)\noutput:\n%s", banned, got)
		}
	}
}

func TestAppsDBTableList_RequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBTableList,
		[]string{"+db-table-list", "--app-id", "  ", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "app-id") {
		t.Fatalf("expected app-id required error, got %v", err)
	}
}

func TestAppsDBTableList_DryRunSendsPaginationAndEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBTableList,
		[]string{"+db-table-list", "--app-id", "app_x", "--env", "dev",
			"--page-size", "50", "--page-token", "cursor-abc",
			"--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "GET" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/tables" {
		t.Fatalf("dry-run method/url = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if env.API[0].Params["env"] != "dev" {
		t.Fatalf("dry-run params.env = %v (want dev)", env.API[0].Params["env"])
	}
	if pz, _ := env.API[0].Params["page_size"].(float64); int(pz) != 50 {
		t.Fatalf("dry-run params.page_size = %v (want 50)", env.API[0].Params["page_size"])
	}
	if env.API[0].Params["page_token"] != "cursor-abc" {
		t.Fatalf("dry-run params.page_token = %v (want cursor-abc)", env.API[0].Params["page_token"])
	}
}

func TestAppsDBTableList_DoesNotSendIncludeStatsQuery(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBTableList,
		[]string{"+db-table-list", "--app-id", "app_x", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := env.API[0].Params["include_stats"]; ok {
		t.Fatalf("CLI should not send include_stats query, but got params=%v", env.API[0].Params)
	}
}

func TestAppsDBTableList_RejectsBadEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBTableList,
		[]string{"+db-table-list", "--app-id", "app_x", "--env", "prod", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "env") {
		t.Fatalf("expected env enum rejection, got %v", err)
	}
}
