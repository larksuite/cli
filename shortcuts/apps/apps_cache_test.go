// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

const (
	cacheURL      = "/open-apis/spark/v1/apps/app_x/cache"
	cacheClearURL = "/open-apis/spark/v1/apps/app_x/cache/clear"
)

// cacheValueStr 是服务端在 wire 上透传的原始 JSON 字符串（value 不反序列化）。
const cacheValueStr = `[{"name":"Alice","award":"Gold"},{"name":"Bob","award":"Silver"}]`

// ── cache-get ──

// TestAppsCacheGet_HitJSON：命中时 json 默认——value 原样透传（不反序列化），
// value_size_bytes 由 CLI 按 value 字节长度算出，environment 取服务端 resolved env。
func TestAppsCacheGet_HitJSON(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"env": "online", "exists": true, "ttl_ms": 272000, "value": cacheValueStr,
		}},
	})
	if err := runAppsShortcut(t, AppsCacheGet,
		[]string{"+cache-get", "--app-id", "app_x", "--environment", "online", "--key", "k:1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	d := parseEnvelopeData(t, stdout)
	if d["key"] != "k:1" || d["environment"] != "online" || d["exists"] != true {
		t.Fatalf("get hit data=%v", d)
	}
	if v, _ := d["value"].(string); v != cacheValueStr {
		t.Fatalf("value must be raw passthrough string, got %v", d["value"])
	}
	if sz, _ := numericAsFloat(d["value_size_bytes"]); int(sz) != len(cacheValueStr) {
		t.Fatalf("value_size_bytes = %v, want %d", d["value_size_bytes"], len(cacheValueStr))
	}
	// ttl_ms 必须是 JSON number（透传服务端数字，不得变成字符串）；JSON 解析后为 float64。
	if _, ok := d["ttl_ms"].(float64); !ok {
		t.Fatalf("ttl_ms must be a JSON number, got %T (%v)", d["ttl_ms"], d["ttl_ms"])
	}
}

// TestAppsCacheGet_HitPretty：pretty 把 value 反序列化后展开（含缩进后的字段），并打元信息标签。
func TestAppsCacheGet_HitPretty(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"env": "online", "exists": true, "ttl_ms": 272000, "value": cacheValueStr,
		}},
	})
	if err := runAppsShortcut(t, AppsCacheGet,
		[]string{"+cache-get", "--app-id", "app_x", "--environment", "online", "--key", "k:1", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"key", "environment", "exists", "value", "Alice"} {
		if !strings.Contains(got, want) {
			t.Errorf("pretty missing %q:\n%s", want, got)
		}
	}
}

// TestAppsCacheGet_Miss：未命中——exists=false，无 value，ttl_ms / value_size_bytes 为 null。
func TestAppsCacheGet_Miss(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"env": "online", "exists": false,
		}},
	})
	if err := runAppsShortcut(t, AppsCacheGet,
		[]string{"+cache-get", "--app-id", "app_x", "--environment", "online", "--key", "k:1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	d := parseEnvelopeData(t, stdout)
	if d["exists"] != false {
		t.Fatalf("miss exists=%v", d["exists"])
	}
	if _, ok := d["value"]; ok {
		t.Fatalf("miss must not carry value: %v", d)
	}
	if d["ttl_ms"] != nil || d["value_size_bytes"] != nil {
		t.Fatalf("miss ttl_ms/value_size_bytes must be null: %v", d)
	}
}

// TestAppsCacheGet_ExistsAsString：服务端把 exists 返成字符串 "true" 时仍按命中处理
// （cacheBool 容错，防 exists 以字符串形态出现被误判成未命中、hit→miss 翻转）。
func TestAppsCacheGet_ExistsAsString(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"env": "online", "exists": "true", "ttl_ms": 272000, "value": cacheValueStr,
		}},
	})
	if err := runAppsShortcut(t, AppsCacheGet,
		[]string{"+cache-get", "--app-id", "app_x", "--environment", "online", "--key", "k:1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	d := parseEnvelopeData(t, stdout)
	if d["exists"] != true {
		t.Fatalf("exists string \"true\" 应按命中解析, got exists=%v", d["exists"])
	}
	if v, _ := d["value"].(string); v != cacheValueStr {
		t.Fatalf("命中应带 value, got %v", d["value"])
	}
}

// TestAppsCacheGet_PrettyNonJSONFallback：pretty 下 value 不是合法 JSON 时降级原样输出
// （safeParseJSON 解析失败→原样打印，不报错、不吞值）。补齐 HitPretty 只覆盖了"能反序列化"路径的缺口。
func TestAppsCacheGet_PrettyNonJSONFallback(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"env": "online", "exists": true, "ttl_ms": 272000, "value": "hello-plain-not-json",
		}},
	})
	if err := runAppsShortcut(t, AppsCacheGet,
		[]string{"+cache-get", "--app-id", "app_x", "--environment", "online", "--key", "k:1", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), "hello-plain-not-json") {
		t.Fatalf("非 JSON value 应原样输出（降级）, got:\n%s", stdout.String())
	}
}

// TestAppsCacheGet_TTLAsStringNormalized：服务端把 ttl_ms 返成字符串 "272000" 时，
// 输出的 ttl_ms 必须归一成 JSON number（cacheInt），不得随 wire 形态漂移成字符串。
func TestAppsCacheGet_TTLAsStringNormalized(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"env": "online", "exists": true, "ttl_ms": "272000", "value": cacheValueStr,
		}},
	})
	if err := runAppsShortcut(t, AppsCacheGet,
		[]string{"+cache-get", "--app-id", "app_x", "--environment", "online", "--key", "k:1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	d := parseEnvelopeData(t, stdout)
	f, ok := d["ttl_ms"].(float64)
	if !ok {
		t.Fatalf("ttl_ms string wire 应归一成 JSON number, got %T (%v)", d["ttl_ms"], d["ttl_ms"])
	}
	if int(f) != 272000 {
		t.Fatalf("ttl_ms = %v, want 272000", f)
	}
}

// TestAppsCacheDelete_CountAsStringNormalized：服务端把 deleted_key_count 返成字符串 "1" 时，
// 输出必须归一成 JSON number（cacheInt）。
func TestAppsCacheDelete_CountAsStringNormalized(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"env": "dev", "deleted_key_count": "1"}},
	})
	if err := runAppsShortcut(t, AppsCacheDelete,
		[]string{"+cache-delete", "--app-id", "app_x", "--environment", "dev", "--key", "k:1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	d := parseEnvelopeData(t, stdout)
	if _, ok := d["deleted_key_count"].(float64); !ok {
		t.Fatalf("deleted_key_count string wire 应归一成 JSON number, got %T (%v)", d["deleted_key_count"], d["deleted_key_count"])
	}
}

// TestAppsCacheGet_DryRunOmitsEnv：不传 --environment 时 dry-run query 不带 env（服务端自动选），但带 key。
func TestAppsCacheGet_DryRunOmitsEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCacheGet,
		[]string{"+cache-get", "--app-id", "app_x", "--key", "k:1", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	a := firstDryRunAPI(t, stdout.String())
	if a.Method != "GET" || a.URL != cacheURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if _, ok := a.Params["env"]; ok {
		t.Fatalf("no --environment → env must be omitted, params=%v", a.Params)
	}
	if a.Params["key"] != "k:1" {
		t.Fatalf("key must be in query, params=%v", a.Params)
	}
}

// TestAppsCacheGet_DryRunWithEnv：显式 --environment dev → query 带 env=dev。
func TestAppsCacheGet_DryRunWithEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCacheGet,
		[]string{"+cache-get", "--app-id", "app_x", "--environment", "dev", "--key", "k:1", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	a := firstDryRunAPI(t, stdout.String())
	if a.Params["env"] != "dev" {
		t.Fatalf("env must be dev, params=%v", a.Params)
	}
}

// TestAppsCacheGet_RequiresKey：缺 --key → 校验错。
func TestAppsCacheGet_RequiresKey(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCacheGet,
		[]string{"+cache-get", "--app-id", "app_x", "--as", "user"}, factory, stdout); err == nil {
		t.Fatalf("expected required --key error")
	}
}

// ── cache-delete ──

// TestAppsCacheDelete_Hit：删中命中的 key → deleted_key_count=1；pretty 打 "✓ cache deleted"。
func TestAppsCacheDelete_Hit(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"env": "dev", "deleted_key_count": 1}},
	})
	if err := runAppsShortcut(t, AppsCacheDelete,
		[]string{"+cache-delete", "--app-id", "app_x", "--environment", "dev", "--key", "k:1", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), "✓ cache deleted") {
		t.Fatalf("pretty: %s", stdout.String())
	}
}

// TestAppsCacheDelete_AbsentJSON：目标不存在 → 幂等成功，deleted_key_count=0，pretty 措辞区分。
func TestAppsCacheDelete_AbsentJSON(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"env": "dev", "deleted_key_count": 0}},
	})
	if err := runAppsShortcut(t, AppsCacheDelete,
		[]string{"+cache-delete", "--app-id", "app_x", "--environment", "dev", "--key", "k:1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	d := parseEnvelopeData(t, stdout)
	if sz, _ := numericAsFloat(d["deleted_key_count"]); int(sz) != 0 || d["key"] != "k:1" || d["environment"] != "dev" {
		t.Fatalf("absent data=%v", d)
	}
}

// TestAppsCacheDelete_AbsentPretty：不存在 pretty 打 "✓ cache already absent"。
func TestAppsCacheDelete_AbsentPretty(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: cacheURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"env": "dev", "deleted_key_count": 0}},
	})
	if err := runAppsShortcut(t, AppsCacheDelete,
		[]string{"+cache-delete", "--app-id", "app_x", "--environment", "dev", "--key", "k:1", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), "already absent") {
		t.Fatalf("pretty: %s", stdout.String())
	}
}

// TestAppsCacheDelete_DryRun：DELETE 方法、/cache 路由，query 带 key + env。
func TestAppsCacheDelete_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCacheDelete,
		[]string{"+cache-delete", "--app-id", "app_x", "--environment", "dev", "--key", "k:1", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	a := firstDryRunAPI(t, stdout.String())
	if a.Method != "DELETE" || a.URL != cacheURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if a.Params["key"] != "k:1" || a.Params["env"] != "dev" {
		t.Fatalf("params=%v", a.Params)
	}
}

// ── cache-clear ──

// TestAppsCacheClear_Success：清空成功 → deleted_key_count=128；pretty 打 "✓ cache cleared: 128 entries (dev)"。
func TestAppsCacheClear_Success(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: cacheClearURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"env": "dev", "deleted_key_count": 128}},
	})
	if err := runAppsShortcut(t, AppsCacheClear,
		[]string{"+cache-clear", "--app-id", "app_x", "--environment", "dev", "--yes", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), "✓ cache cleared: 128 entries (dev)") {
		t.Fatalf("pretty: %s", stdout.String())
	}
}

// TestAppsCacheClear_RequiresConfirmation：high-risk-write 无 --yes → 被确认门拦截。
func TestAppsCacheClear_RequiresConfirmation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCacheClear,
		[]string{"+cache-clear", "--app-id", "app_x", "--environment", "dev", "--as", "user"}, factory, stdout); err == nil {
		t.Fatalf("expected confirmation gate without --yes")
	}
}

// TestAppsCacheClear_DryRunBodyWithEnv：dry-run POST /cache/clear，body 带 env=dev。
func TestAppsCacheClear_DryRunBodyWithEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCacheClear,
		[]string{"+cache-clear", "--app-id", "app_x", "--environment", "dev", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	a := firstDryRunAPI(t, stdout.String())
	if a.Method != "POST" || a.URL != cacheClearURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if a.Body["env"] != "dev" {
		t.Fatalf("body must carry env=dev, body=%v", a.Body)
	}
}

// TestAppsCacheClear_DryRunBodyOmitsEnv：不传 --environment → body 不带 env（服务端自动选）。
func TestAppsCacheClear_DryRunBodyOmitsEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCacheClear,
		[]string{"+cache-clear", "--app-id", "app_x", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	a := firstDryRunAPI(t, stdout.String())
	if _, ok := a.Body["env"]; ok {
		t.Fatalf("no --environment → body env must be omitted, body=%v", a.Body)
	}
}

// firstDryRunAPI 解析 dry-run 输出的第一个 api[] 项（method/url/params/body）。
// 复用本包规范的 dryRunAPIEnvelope（api 现嵌在 data.api 下，见 dryrun_test.go）。
func firstDryRunAPI(t *testing.T, s string) dryRunAPICall {
	t.Helper()
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(s), &env); err != nil || len(env.API) == 0 {
		t.Fatalf("bad dry-run json: %v\n%s", err, s)
	}
	return env.API[0]
}
