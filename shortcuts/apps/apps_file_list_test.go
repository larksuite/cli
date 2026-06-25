// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

// 设计原则三：<timestamp> 四种格式 → 统一 RFC3339 UTC。
func TestNormalizeTimestamp_AllFormats(t *testing.T) {
	// 空串透传
	if got, err := normalizeTimestamp("  "); err != nil || got != "" {
		t.Fatalf("empty → %q,%v want \"\",nil", got, err)
	}

	// ISO 8601 带 TZ：Z 原样、显式偏移换算到 UTC
	mustEq := func(in, want string) {
		got, err := normalizeTimestamp(in)
		if err != nil || got != want {
			t.Errorf("normalizeTimestamp(%q)=%q,%v want %q", in, got, err, want)
		}
	}
	mustEq("2026-04-15T10:00:00Z", "2026-04-15T10:00:00Z")
	mustEq("2026-04-15T10:00:00+08:00", "2026-04-15T02:00:00Z") // +08:00 → UTC -8h

	// date / local datetime：按本地时区解释再转 UTC（与 time.ParseInLocation 对齐）
	dExp, _ := time.ParseInLocation("2006-01-02", "2026-04-15", time.Local)
	mustEq("2026-04-15", dExp.UTC().Format(time.RFC3339))
	ldExp, _ := time.ParseInLocation("2006-01-02T15:04:05", "2026-04-15T10:00:00", time.Local)
	mustEq("2026-04-15T10:00:00", ldExp.UTC().Format(time.RFC3339))

	// 相对：从现在往前推，结果应 ≈ now-dur（5s 容差）
	for _, c := range []struct {
		in  string
		dur time.Duration
	}{{"30s", 30 * time.Second}, {"5m", 5 * time.Minute}, {"2h", 2 * time.Hour}, {"3d", 72 * time.Hour}, {"1w", 7 * 24 * time.Hour}} {
		got, err := normalizeTimestamp(c.in)
		if err != nil {
			t.Errorf("normalizeTimestamp(%q) err=%v", c.in, err)
			continue
		}
		ts, perr := time.Parse(time.RFC3339, got)
		if perr != nil {
			t.Errorf("normalizeTimestamp(%q)=%q not RFC3339", c.in, got)
			continue
		}
		want := time.Now().Add(-c.dur)
		if diff := want.Sub(ts); diff > 5*time.Second || diff < -5*time.Second {
			t.Errorf("normalizeTimestamp(%q)=%q off by %v from now-%v", c.in, got, diff, c.dur)
		}
	}

	// 非法格式 → error
	for _, bad := range []string{"notatime", "7x", "2026/04/15", "2026-13-99"} {
		if _, err := normalizeTimestamp(bad); err == nil {
			t.Errorf("normalizeTimestamp(%q) expected error", bad)
		}
	}
}

const fileListURL = "/open-apis/spark/v1/apps/app_x/storage/file_list"

// TestAppsFileList_RequiresAppID 验证空白 --app-id 触发 --app-id typed 校验错误。
func TestAppsFileList_RequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsFileList,
		[]string{"+file-list", "--app-id", "  ", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--app-id" {
		t.Fatalf("Param = %q, want --app-id", ve.Param)
	}
}

// 过滤器 + 分页全部进 query（size-gt/lt 走 int，uploaded_since/until 原样）。
func TestAppsFileList_DryRunSendsFiltersAndPagination(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsFileList,
		[]string{"+file-list", "--app-id", "app_x",
			"--name", "logo.png", "--path", "/x.png", "--type", "image/png",
			"--size-gt", "100", "--size-lt", "9000",
			"--uploaded-since", "2026-01-01", "--uploaded-until", "2026-02-01",
			"--page-size", "5", "--page-token", "cur-1",
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
	a := env.API[0]
	if a.Method != "GET" || a.URL != fileListURL {
		t.Fatalf("method/url = %s %s", a.Method, a.URL)
	}
	// 设计原则三：date 入参会被归一化为 RFC3339 UTC，期望值用 normalizeTimestamp 计算（避开本地时区脆弱断言）。
	sinceN, _ := normalizeTimestamp("2026-01-01")
	untilN, _ := normalizeTimestamp("2026-02-01")
	wantStr := map[string]string{
		"name": "logo.png", "path": "/x.png", "type": "image/png",
		"uploaded_since": sinceN, "uploaded_until": untilN, "page_token": "cur-1",
	}
	for k, v := range wantStr {
		if a.Params[k] != v {
			t.Errorf("params.%s = %v, want %v", k, a.Params[k], v)
		}
	}
	// 且确实归一化成了 UTC（以 Z 结尾），不是原样透传。
	if s, _ := a.Params["uploaded_since"].(string); !strings.HasSuffix(s, "Z") {
		t.Errorf("uploaded_since not normalized to RFC3339 UTC: %v", a.Params["uploaded_since"])
	}
	for _, k := range []string{"size_gt", "size_lt", "page_size"} {
		if _, ok := a.Params[k]; !ok {
			t.Errorf("params missing %s: %v", k, a.Params)
		}
	}
}

// 0 值过滤器不下发（size-gt/lt 缺省 0、空字符串过滤器）。
func TestAppsFileList_DryRunOmitsEmptyFilters(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsFileList,
		[]string{"+file-list", "--app-id", "app_x", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	for _, banned := range []string{"name", "path", "type", "size_gt", "size_lt", "uploaded_since", "uploaded_until", "page_token"} {
		if _, ok := env.API[0].Params[banned]; ok {
			t.Errorf("params should omit empty %s: %v", banned, env.API[0].Params)
		}
	}
	if _, ok := env.API[0].Params["page_size"]; !ok {
		t.Errorf("params should always carry page_size: %v", env.API[0].Params)
	}
}

// created_at/created_by → uploaded_at/uploaded_by；created_by 是 JSON 字符串 → parse 成对象。
func TestAppsFileList_SuccessProjectsCreatedToUploaded(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    fileListURL,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":   false,
				"page_token": "",
				"items": []interface{}{
					map[string]interface{}{
						"file_name":    "logo.png",
						"path":         "/1858537546760216.png",
						"size_bytes":   24580,
						"type":         "image/png",
						"created_at":   "2026-04-15T10:30:00Z",
						"created_by":   `{"id":"7311","name":"alice"}`,
						"download_url": "/spark/app/x/1858537546760216.png",
					},
				},
			},
		},
	})
	if err := runAppsShortcut(t, AppsFileList,
		[]string{"+file-list", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{`"uploaded_at": "2026-04-15T10:30:00Z"`, `"uploaded_by"`, `"name": "alice"`, `"id": "7311"`} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout missing %q:\n%s", want, got)
		}
	}
	// created_* 不应再出现在输出。
	for _, banned := range []string{"created_at", "created_by"} {
		if strings.Contains(got, banned) {
			t.Errorf("stdout should not contain %q (renamed to uploaded_*):\n%s", banned, got)
		}
	}
}

// TestAppsFileList_PrettyTableAndEmpty 验证 pretty 非空时渲染表头与人类可读 size，空结果时输出 "No files found."。
func TestAppsFileList_PrettyTableAndEmpty(t *testing.T) {
	// 非空：5 列表头。
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: fileListURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"items": []interface{}{map[string]interface{}{
				"file_name": "logo.png", "path": "/x.png", "size_bytes": 24576, "type": "image/png",
				"created_at": "2026-04-15T10:30:00Z",
			}},
		}},
	})
	if err := runAppsShortcut(t, AppsFileList,
		[]string{"+file-list", "--app-id", "app_x", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "file_name") || !strings.Contains(got, "uploaded_at") || !strings.Contains(got, "24 KB") {
		t.Fatalf("pretty table malformed:\n%s", got)
	}

	// 空：No files found.
	factory2, stdout2, reg2 := newAppsExecuteFactory(t)
	reg2.Register(&httpmock.Stub{
		Method: "GET", URL: fileListURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{}}},
	})
	if err := runAppsShortcut(t, AppsFileList,
		[]string{"+file-list", "--app-id", "app_x", "--format", "pretty", "--as", "user"}, factory2, stdout2); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout2.String(), "No files found.") {
		t.Fatalf("empty pretty should say 'No files found.', got: %s", stdout2.String())
	}
}

// TestParseFileUser_Cases 验证 parseFileUser：合法 JSON 解析成对象，空串/非法/全空字段均返回 nil。
func TestParseFileUser_Cases(t *testing.T) {
	if u := parseFileUser(`{"id":"1","name":"a"}`); u == nil || u.ID != "1" || u.Name != "a" {
		t.Fatalf("valid parse failed: %#v", u)
	}
	if u := parseFileUser(""); u != nil {
		t.Errorf("empty → nil, got %#v", u)
	}
	if u := parseFileUser("not json"); u != nil {
		t.Errorf("invalid → nil, got %#v", u)
	}
	if u := parseFileUser(`{"id":"","name":""}`); u != nil {
		t.Errorf("all-empty → nil, got %#v", u)
	}
}
