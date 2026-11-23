// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsLogList_DryRunBuildsSearchLogsBody(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsLogList, []string{
		"+log-list", "--app-id", "app_x", "--level", "error",
		"--trace-id", "trace-1",
		"--keyword", "timeout", "--module", "frontend", "--user-id", "ou_1",
		"--page", "/home", "--api", "/api/orders", "--min-duration", "200",
		"--since", "2026-06-23T10:00:00Z", "--until", "2026-06-23T10:01:00Z",
		"--page-size", "20", "--dry-run", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "POST" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/search_logs" {
		t.Fatalf("method/url = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if env.API[0].Body["app_env"] != "runtime" || env.API[0].Body["limit"] != float64(20) {
		t.Fatalf("body = %#v", env.API[0].Body)
	}
	filter := env.API[0].Body["filter"].(map[string]interface{})
	if got := filter["keyword"]; got != "timeout" {
		t.Fatalf("filter.keyword = %v", got)
	}
	for key, want := range map[string]string{
		"modules":  "frontend",
		"user_ids": "ou_1",
		"pages":    "/home",
		"apis":     "/api/orders",
	} {
		values, ok := filter[key].([]interface{})
		if !ok || len(values) != 1 || values[0] != want {
			t.Fatalf("filter.%s = %#v, want [%q]", key, filter[key], want)
		}
	}
	if env.API[0].Body["start_timestamp_ns"] != "1782208800000000000" ||
		env.API[0].Body["end_timestamp_ns"] != "1782208860000000000" {
		t.Fatalf("timestamps = %#v %#v", env.API[0].Body["start_timestamp_ns"], env.API[0].Body["end_timestamp_ns"])
	}
}

func TestAppsLogList_DoesNotAcceptLogIDFlag(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsLogList, []string{
		"+log-list", "--app-id", "app_x", "--log-id", "LOG1", "--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --log-id") {
		t.Fatalf("expected unknown --log-id flag, got %v", err)
	}
}

func TestAppsLogList_RejectsDevEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsLogList, []string{"+log-list", "--app-id", "app_x", "--environment", "dev", "--as", "user"}, factory, stdout)
	requireAppsValidationParam(t, err, "--environment")
}

func TestAppsLogGet_SearchesByLogIDLimitOne(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{"log_id": "LOG1", "level": "INFO"},
				},
			},
		},
	}
	reg.Register(stub)
	if err := runAppsShortcut(t, AppsLogGet, []string{"+log-get", "--app-id", "app_x", "--log-id", "LOG1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["limit"] != float64(1) {
		t.Fatalf("limit = %v, want 1", sent["limit"])
	}
	if sent["app_env"] != "runtime" {
		t.Fatalf("app_env = %v, want runtime", sent["app_env"])
	}
}

func TestAppsLogGet_AcceptsDataArraySearchResponse(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	search := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		RawBody: []byte(`{
			"code": 0,
			"data": [
				{
					"log_id": "LOG7655249917057764881",
					"level": "ERROR",
					"attributes": {
						"commit_id": "commit_array",
						"source_map_file_prefix": "sourcemaps/array",
						"frames": [{"file":"main.js","line":10,"column":20}]
					}
				}
			]
		}`),
	}
	resolve := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/resolve_stack_trace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"source_stack": []interface{}{
					map[string]interface{}{"file": "src/App.tsx", "line": 7, "column": 9},
				},
			},
		},
	}
	reg.Register(search)
	reg.Register(resolve)

	if err := runAppsShortcut(t, AppsLogGet, []string{"+log-get", "--app-id", "app_x", "--log-id", "LOG7655249917057764881", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"source_stack_status": "resolved"`) || !strings.Contains(got, "src/App.tsx") {
		t.Fatalf("stdout missing resolved source stack from data array response: %s", got)
	}
}

func TestAppsLogList_NormalizesResponseVariantsAndCanonicalLevel(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"logItems": []interface{}{
					map[string]interface{}{
						"id":           "LOG1",
						"traceID":      "trace-1",
						"timestampNs":  "1782209472123456789",
						"severityText": "ERROR",
					},
				},
				"nextPageToken": "tok-next",
				"hasMore":       true,
			},
		},
	})

	if err := runAppsShortcut(t, AppsLogList, []string{"+log-list", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items     []map[string]interface{} `json:"items"`
			PageToken string                   `json:"page_token"`
			HasMore   bool                     `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if env.Data.PageToken != "tok-next" || !env.Data.HasMore {
		t.Fatalf("pagination = token %q has_more %v", env.Data.PageToken, env.Data.HasMore)
	}
	if len(env.Data.Items) != 1 {
		t.Fatalf("items len = %d", len(env.Data.Items))
	}
	item := env.Data.Items[0]
	if item["level"] != "ERROR" || item["severity_text"] != "ERROR" || item["severityText"] != "ERROR" {
		t.Fatalf("level fields = %#v", item)
	}
}

func TestAppsLogList_NormalizesKVAttributesToObject(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{
						"log_id": "LOG1",
						"attributes": []interface{}{
							map[string]interface{}{"key": "app_env", "value": "runtime"},
							map[string]interface{}{"key": "duration_ms", "value": "8263"},
							map[string]interface{}{"key": "module", "value": "gateway"},
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsLogList, []string{"+log-list", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items []map[string]interface{} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	attrs, ok := env.Data.Items[0]["attributes"].(map[string]interface{})
	if !ok {
		t.Fatalf("attributes = %#v, want object", env.Data.Items[0]["attributes"])
	}
	if attrs["app_env"] != "runtime" || attrs["duration_ms"] != "8263" || attrs["module"] != "gateway" {
		t.Fatalf("attributes = %#v", attrs)
	}
}

func TestAppsLogGet_PrettyFormatsTimestamp(t *testing.T) {
	const rawNS = int64(1782209472123456789)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{
						"log_id":       "LOG1",
						"level":        "ERROR",
						"trace_id":     "trace-1",
						"timestamp_ns": rawNS,
						"message":      "boom",
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsLogGet, []string{
		"+log-get", "--app-id", "app_x", "--log-id", "LOG1", "--format", "pretty", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	wantTime := time.Unix(0, rawNS).Local().Format("2006-01-02 15:04:05.000")
	if !strings.HasPrefix(got, "time") {
		t.Fatalf("pretty output should start with time column, got:\n%s", got)
	}
	if !strings.Contains(got, wantTime) {
		t.Fatalf("pretty output missing formatted time %q:\n%s", wantTime, got)
	}
	if strings.Contains(got, "timestamp_ns") || strings.Contains(got, "1782209472123456789") {
		t.Fatalf("pretty output should hide raw timestamp_ns, got:\n%s", got)
	}
}

func TestAppsLogGet_ResolvesSourceStackWhenFieldsPresent(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	search := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{
						"log_id": "LOG1",
						"level":  "ERROR",
						"attributes": map[string]interface{}{
							"commit_id":              "commit_1",
							"source_map_file_prefix": "sourcemaps/app",
							"frames": []interface{}{
								map[string]interface{}{"file": "main.js", "line": 10, "column": 20},
							},
						},
					},
				},
			},
		},
	}
	resolve := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/resolve_stack_trace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"source_stack": []interface{}{
					map[string]interface{}{"file": "src/App.tsx", "line": 7, "column": 9},
				},
			},
		},
	}
	reg.Register(search)
	reg.Register(resolve)

	if err := runAppsShortcut(t, AppsLogGet, []string{"+log-get", "--app-id", "app_x", "--log-id", "LOG1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(resolve.CapturedBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["commit_id"] != "commit_1" || sent["source_map_file_prefix"] != "sourcemaps/app" {
		t.Fatalf("resolve body missing source map fields: %#v", sent)
	}
	frames, ok := sent["frames"].([]interface{})
	if !ok || len(frames) != 1 {
		t.Fatalf("resolve frames = %#v", sent["frames"])
	}
	if got := stdout.String(); !strings.Contains(got, `"source_stack_status": "resolved"`) || !strings.Contains(got, "src/App.tsx") {
		t.Fatalf("stdout missing resolved source stack: %s", got)
	}
}

func TestAppsLogGet_ResolvesSourceStackFromNestedKVAttributes(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	search := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{
						"log_id":       "LOG7655249917057764881",
						"severityText": "ERROR",
						"attributes": []interface{}{
							map[string]interface{}{"key": "commit_id", "value": "commit_nested"},
							map[string]interface{}{"key": "source_map_file_prefix", "value": "sourcemaps/nested"},
							map[string]interface{}{
								"key": "exception",
								"value": map[string]interface{}{
									"stackTrace": strings.Join([]string{
										"TypeError: failed to render",
										"    at render (https://cdn.example.com/assets/main.js:12:34)",
										"    at https://cdn.example.com/assets/chunk.js:56:78",
									}, "\n"),
								},
							},
						},
					},
				},
			},
		},
	}
	resolve := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/resolve_stack_trace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"source_stack": []interface{}{
					map[string]interface{}{"file": "src/App.tsx", "line": 12, "column": 34},
				},
			},
		},
	}
	reg.Register(search)
	reg.Register(resolve)

	if err := runAppsShortcut(t, AppsLogGet, []string{"+log-get", "--app-id", "app_x", "--log-id", "LOG7655249917057764881", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(resolve.CapturedBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["commit_id"] != "commit_nested" || sent["source_map_file_prefix"] != "sourcemaps/nested" {
		t.Fatalf("resolve body missing nested source map fields: %#v", sent)
	}
	frames, ok := sent["frames"].([]interface{})
	if !ok || len(frames) != 2 {
		t.Fatalf("resolve frames = %#v, want parsed stack frames", sent["frames"])
	}
	frame, ok := frames[0].(map[string]interface{})
	if !ok {
		t.Fatalf("parsed frame = %#v, want object", frames[0])
	}
	if frame["function"] != "render" || frame["file_name"] != "main.js" || frame["line"] != float64(12) || frame["column"] != float64(34) {
		t.Fatalf("parsed frame = %#v", frame)
	}
	bare, ok := frames[1].(map[string]interface{})
	if !ok {
		t.Fatalf("bare frame = %#v, want object", frames[1])
	}
	if bare["file_name"] != "chunk.js" || bare["line"] != float64(56) || bare["column"] != float64(78) {
		t.Fatalf("bare frame = %#v", bare)
	}
	if got := stdout.String(); !strings.Contains(got, `"source_stack_status": "resolved"`) || !strings.Contains(got, "src/App.tsx") {
		t.Fatalf("stdout missing resolved source stack: %s", got)
	}
}

func TestAppsLogGet_ResolvesSourceStackFromReleaseCommitJSONStack(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	search := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{
						"log_id":       "LOG7655249917057764881",
						"severityText": "ERROR",
						"attributes": map[string]interface{}{
							"tenant_id":         "110564",
							"release_commit_id": "4b393e4e0ca9ca1a855ba4585bc6750a7db2266f",
							"stack": `[{"fileName":"main.js","line":3348,"column":540585},` +
								`{"fileName":"main.js","line":3107,"column":51935},` +
								`{"fileName":"main.js","line":62,"column":12516}]`,
						},
					},
				},
			},
		},
	}
	resolve := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/resolve_stack_trace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"source_stack": []interface{}{
					map[string]interface{}{"file": "src/App.tsx", "line": 42, "column": 7},
				},
			},
		},
	}
	reg.Register(search)
	reg.Register(resolve)

	if err := runAppsShortcut(t, AppsLogGet, []string{"+log-get", "--app-id", "app_x", "--log-id", "LOG7655249917057764881", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(resolve.CapturedBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["commit_id"] != "4b393e4e0ca9ca1a855ba4585bc6750a7db2266f" || sent["source_map_file_prefix"] != defaultSourceMapPrefix || sent["tenant_id"] != "110564" {
		t.Fatalf("resolve body missing release source map fields: %#v", sent)
	}
	frames, ok := sent["frames"].([]interface{})
	if !ok || len(frames) != 3 {
		t.Fatalf("resolve frames = %#v, want all valid generated frames", sent["frames"])
	}
	first, ok := frames[0].(map[string]interface{})
	if !ok {
		t.Fatalf("first frame = %#v, want object", frames[0])
	}
	if first["file_name"] != "main.js" || first["line"] != float64(3348) || first["column"] != float64(540585) {
		t.Fatalf("first frame = %#v", first)
	}
	if got := stdout.String(); !strings.Contains(got, `"source_stack_status": "resolved"`) || !strings.Contains(got, "src/App.tsx") {
		t.Fatalf("stdout missing resolved source stack: %s", got)
	}
}

func TestAppsLogGet_ResolvesSourceStackFromJSONBodyStack(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	search := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{
						"log_id":       "LOG_BODY_STACK",
						"severityText": "ERROR",
						"attributes": map[string]interface{}{
							"release_commit_id": "commit_body",
						},
						"body": `{"error":{"stack":"AxiosError: failed\n    at request (https://cdn.example.com/client/assets/body.js:9:88)"}}`,
					},
				},
			},
		},
	}
	resolve := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/resolve_stack_trace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"source_stack": []interface{}{
					map[string]interface{}{"file": "src/request.ts", "line": 9, "column": 88},
				},
			},
		},
	}
	reg.Register(search)
	reg.Register(resolve)

	if err := runAppsShortcut(t, AppsLogGet, []string{"+log-get", "--app-id", "app_x", "--log-id", "LOG_BODY_STACK", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(resolve.CapturedBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["commit_id"] != "commit_body" || sent["source_map_file_prefix"] != defaultSourceMapPrefix {
		t.Fatalf("resolve body missing body stack source map fields: %#v", sent)
	}
	frames, ok := sent["frames"].([]interface{})
	if !ok || len(frames) != 1 {
		t.Fatalf("resolve frames = %#v, want parsed JSON body stack frame", sent["frames"])
	}
	frame, ok := frames[0].(map[string]interface{})
	if !ok {
		t.Fatalf("frame = %#v, want object", frames[0])
	}
	if frame["function"] != "request" || frame["file_name"] != "body.js" || frame["line"] != float64(9) || frame["column"] != float64(88) {
		t.Fatalf("frame = %#v", frame)
	}
	if got := stdout.String(); !strings.Contains(got, `"source_stack_status": "resolved"`) || !strings.Contains(got, "src/request.ts") {
		t.Fatalf("stdout missing resolved source stack: %s", got)
	}
}

func TestAppsLogGet_SourceStackMissingFieldsDoesNotFail(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	search := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{
						"log_id":     "LOG1",
						"level":      "ERROR",
						"message":    "TypeError at https://cdn.example.com/main.js:10:20",
						"attributes": map[string]interface{}{"commit_id": "commit_1"},
					},
				},
			},
		},
	}
	reg.Register(search)

	if err := runAppsShortcut(t, AppsLogGet, []string{"+log-get", "--app-id", "app_x", "--log-id", "LOG1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"log_id": "LOG1"`) {
		t.Fatalf("stdout missing original log: %s", got)
	} else if !strings.Contains(got, `"source_stack_status": "unresolved"`) {
		t.Fatalf("stdout missing unresolved source stack status: %s", got)
	} else if !strings.Contains(got, `"source_stack_reason"`) {
		t.Fatalf("stdout missing sanitized source stack reason: %s", got)
	}
	for _, banned := range []string{"secret", "token", "raw request payload"} {
		if strings.Contains(strings.ToLower(stdout.String()), banned) {
			t.Fatalf("stdout leaked %q: %s", banned, stdout.String())
		}
	}
}

func TestAppsLogGet_ErrorNonFrontendMissingFieldsDoesNotMarkUnresolved(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{
						"log_id":  "LOG1",
						"level":   "ERROR",
						"message": "go stack trace: database query failed",
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsLogGet, []string{"+log-get", "--app-id", "app_x", "--log-id", "LOG1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); strings.Contains(got, "source_stack_status") {
		t.Fatalf("non-frontend error log should not be marked unresolved: %s", got)
	}
}

func TestAppsLogGet_SourceStackResolveFailureIsRedacted(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	search := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_logs",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"log_items": []interface{}{
					map[string]interface{}{
						"log_id": "LOG1",
						"level":  "ERROR",
						"attributes": map[string]interface{}{
							"commit_id":              "commit_1",
							"source_map_file_prefix": "sourcemaps/app",
							"frames": []interface{}{
								map[string]interface{}{"file": "main.js", "line": 10, "column": 20},
							},
						},
					},
				},
			},
		},
	}
	resolve := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/resolve_stack_trace",
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "secret token raw request payload should be redacted",
		},
	}
	reg.Register(search)
	reg.Register(resolve)

	if err := runAppsShortcut(t, AppsLogGet, []string{"+log-get", "--app-id", "app_x", "--log-id", "LOG1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"source_stack_status": "unresolved"`) {
		t.Fatalf("stdout missing unresolved status: %s", got)
	}
	if !strings.Contains(got, `"source_stack_error_code": 999`) {
		t.Fatalf("stdout missing resolve error code: %s", got)
	}
	for _, banned := range []string{"secret", "token", "raw request payload"} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Fatalf("stdout leaked %q: %s", banned, got)
		}
	}
}
