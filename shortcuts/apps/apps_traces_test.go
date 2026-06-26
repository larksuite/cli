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

func TestAppsTraceList_DryRunBuildsSearchTracesBody(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsTraceList, []string{
		"+trace-list", "--app-id", "app_x", "--trace-id", "trace-1",
		"--root-span", "gateway", "--user-id", "ou_1",
		"--since", "2026-06-23T10:00:00Z", "--until", "2026-06-23T10:01:00Z",
		"--page-size", "10", "--dry-run", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "POST" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/search_traces" {
		t.Fatalf("method/url = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if env.API[0].Body["app_env"] != "runtime" || env.API[0].Body["limit"] != float64(10) {
		t.Fatalf("body = %#v", env.API[0].Body)
	}
	filter := env.API[0].Body["filter"].(map[string]interface{})
	traceIDs := filter["trace_ids"].([]interface{})
	if len(traceIDs) != 1 || traceIDs[0] != "trace-1" {
		t.Fatalf("filter.trace_ids = %#v", traceIDs)
	}
	if got := filter["keyword"]; got != "gateway" {
		t.Fatalf("filter.keyword = %v", got)
	}
	userIDs := filter["user_ids"].([]interface{})
	if len(userIDs) != 1 || userIDs[0] != "ou_1" {
		t.Fatalf("filter.user_ids = %#v", userIDs)
	}
	if env.API[0].Body["start_timestamp_ns"] != "1782208800000000000" ||
		env.API[0].Body["end_timestamp_ns"] != "1782208860000000000" {
		t.Fatalf("timestamps = %#v %#v", env.API[0].Body["start_timestamp_ns"], env.API[0].Body["end_timestamp_ns"])
	}
}

func TestAppsTraceList_RejectsDevEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsTraceList, []string{"+trace-list", "--app-id", "app_x", "--environment", "dev", "--as", "user"}, factory, stdout)
	requireAppsValidationParam(t, err, "--environment")
}

func TestAppsTraceGet_DryRunBuildsGetTraceBody(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsTraceGet, []string{
		"+trace-get", "--app-id", "app_x", "--trace-id", "trace-1", "--dry-run", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "POST" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/trace" {
		t.Fatalf("method/url = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if env.API[0].Body["app_env"] != "runtime" || env.API[0].Body["trace_id"] != "trace-1" {
		t.Fatalf("body = %#v", env.API[0].Body)
	}
}

func TestNormalizeTraceSummaries_DeduplicatesSpanList(t *testing.T) {
	got := normalizeTraceSummaries([]map[string]interface{}{
		{"trace_id": "trace-1", "name": "gateway"},
		{"traceId": "trace-1", "name": "handler"},
	})
	if len(got) != 1 {
		t.Fatalf("summaries len = %d, want 1: %#v", len(got), got)
	}
	if got[0]["trace_id"] != "trace-1" || got[0]["span_count"] != 2 {
		t.Fatalf("summary = %#v", got[0])
	}
}

func TestNormalizeTraceSummaries_PrefersRootCandidateOverChildOrder(t *testing.T) {
	got := normalizeTraceSummaries([]map[string]interface{}{
		{
			"trace_id":       "trace-1",
			"parent_span_id": "span-root",
			"name":           "child",
			"status":         "ERROR",
			"start_time_ns":  "200",
			"duration_ms":    10,
		},
		{
			"traceID":        "trace-1",
			"parentSpanID":   "",
			"spanName":       "root",
			"status":         "OK",
			"startTimeNs":    "100",
			"durationMs":     200,
			"userID":         "ou_root",
			"parent_span_id": "",
		},
	})
	if len(got) != 1 {
		t.Fatalf("summaries len = %d, want 1: %#v", len(got), got)
	}
	summary := got[0]
	if summary["trace_id"] != "trace-1" || summary["span_count"] != 2 {
		t.Fatalf("summary identity/count = %#v", summary)
	}
	if summary["root_span"] != "root" {
		t.Fatalf("root_span = %#v, want root: %#v", summary["root_span"], summary)
	}
	if summary["status"] != "ERROR" {
		t.Fatalf("status = %#v, want ERROR: %#v", summary["status"], summary)
	}
	if summary["start_time_ns"] != "100" {
		t.Fatalf("start_time_ns = %#v, want earliest 100: %#v", summary["start_time_ns"], summary)
	}
	if summary["duration_ms"] != 200 {
		t.Fatalf("duration_ms = %#v, want max 200: %#v", summary["duration_ms"], summary)
	}
	if summary["user_id"] != "ou_root" {
		t.Fatalf("user_id = %#v, want root candidate user: %#v", summary["user_id"], summary)
	}
}

func TestAppsTraceList_NormalizesTraceItemsPaginationVariants(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_traces",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"traceItems": []interface{}{
					map[string]interface{}{
						"traceID":     "trace-1",
						"startTimeNs": "1782209472123456789",
						"rootSpan":    "gateway",
						"userID":      "ou_1",
						"durationMs":  float64(123),
						"spanCount":   float64(7),
					},
				},
				"nextPageToken": "tok-next",
				"hasMore":       true,
			},
		},
	})

	if err := runAppsShortcut(t, AppsTraceList, []string{"+trace-list", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
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
	if item["trace_id"] != "trace-1" || item["root_span"] != "gateway" || item["user_id"] != "ou_1" {
		t.Fatalf("item aliases = %#v", item)
	}
	if item["span_count"] != float64(7) {
		t.Fatalf("span_count = %#v", item["span_count"])
	}
}

func TestAppsTraceList_AggregatesSpansSourceWithSingleSpanPerTrace(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_traces",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"spans": []interface{}{
					map[string]interface{}{
						"traceID": "trace-1",
						"name":    "gateway",
					},
					map[string]interface{}{
						"trace_id":  "trace-2",
						"span_name": "worker",
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsTraceList, []string{"+trace-list", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
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
	if len(env.Data.Items) != 2 {
		t.Fatalf("items len = %d, want 2: %#v", len(env.Data.Items), env.Data.Items)
	}
	wantRootSpan := map[string]string{
		"trace-1": "gateway",
		"trace-2": "worker",
	}
	for _, item := range env.Data.Items {
		traceID, ok := item["trace_id"].(string)
		if !ok || traceID == "" {
			t.Fatalf("missing canonical trace_id: %#v", item)
		}
		if item["span_count"] != float64(1) {
			t.Fatalf("span_count for %s = %#v, want 1: %#v", traceID, item["span_count"], item)
		}
		if item["root_span"] != wantRootSpan[traceID] {
			t.Fatalf("root_span for %s = %#v, want %q: %#v", traceID, item["root_span"], wantRootSpan[traceID], item)
		}
	}
}

func TestAppsTraceList_PrettyUsesTraceSummaryColumns(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/search_traces",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"traceItems": []interface{}{
					map[string]interface{}{
						"traceID":     "trace-1",
						"startTimeNs": "1782232472381701316",
						"rootSpan":    "GET /app/app_x/api/note-records",
						"userID":      "1846640196867498",
						"durationMs":  float64(414),
						"status":      "OK",
						"spanCount":   float64(4),
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsTraceList, []string{
		"+trace-list", "--app-id", "app_x", "--format", "pretty", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.HasPrefix(got, "start-time") {
		t.Fatalf("pretty output should start with start-time column, got:\n%s", got)
	}
	for _, want := range []string{"root-span", "user-id", "duration", "trace-id", "GET /app/app_x/api/note-records", "414ms"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"span_count", "span-count", "status", "duration_ms", "root_span", "trace_id"} {
		if strings.Contains(got, banned) {
			t.Fatalf("pretty output should not include %q:\n%s", banned, got)
		}
	}
}

func TestAppsTraceGet_PrettySummarizesSpans(t *testing.T) {
	const rawNS = int64(1782232472381701316)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/trace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"is_break": false,
				"spans": []interface{}{
					map[string]interface{}{
						"trace_id":             "trace-1",
						"name":                 "GET /app/app_x",
						"span_id":              "root",
						"parent_span_id":       "",
						"start_time_unix_nano": "1782232472381701316",
						"end_time_unix_nano":   "1782232480645457992",
						"attributes": []interface{}{
							map[string]interface{}{"key": "duration_ms", "value": "8263.76"},
							map[string]interface{}{"key": "user_id", "value": "1826968659245100"},
						},
					},
					map[string]interface{}{
						"trace_id":             "trace-1",
						"name":                 "child",
						"span_id":              "child",
						"parent_span_id":       "root",
						"start_time_unix_nano": "1782232480448000000",
						"attributes": []interface{}{
							map[string]interface{}{"key": "duration_ms", "value": "184.89"},
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsTraceGet, []string{
		"+trace-get", "--app-id", "app_x", "--trace-id", "trace-1", "--format", "pretty", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	wantTime := time.Unix(0, rawNS).Local().Format("2006-01-02 15:04:05.000")
	if !strings.HasPrefix(got, "start-time") {
		t.Fatalf("pretty output should start with start-time columns, got:\n%s", got)
	}
	for _, want := range []string{"root-span", "user-id", "duration", "trace-id", "trace-1", "GET /app/app_x", "1826968659245100", wantTime} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"start_time_ns", "1782232472381701316", "span_count", "span-count", "status", "duration_ms", "root_span", "trace_id"} {
		if strings.Contains(got, banned) {
			t.Fatalf("pretty output should not include %q:\n%s", banned, got)
		}
	}
}

func TestAppsTraceGet_NormalizesTraceDetailCamelFields(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/trace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"traceDetail": map[string]interface{}{
					"traceID": "trace-1",
					"isBreak": true,
					"spans": []interface{}{
						map[string]interface{}{
							"spanID":       "span-1",
							"parentSpanID": "root",
							"traceID":      "trace-1",
							"startTimeNs":  "1782209472123456789",
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsTraceGet, []string{"+trace-get", "--app-id", "app_x", "--trace-id", "trace-1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if _, wrapped := env.Data["trace"]; wrapped {
		t.Fatalf("trace-get should output the trace object directly: %#v", env.Data)
	}
	if env.Data["trace_id"] != "trace-1" || env.Data["is_break"] != true {
		t.Fatalf("trace aliases = %#v", env.Data)
	}
	spans := env.Data["spans"].([]interface{})
	span := spans[0].(map[string]interface{})
	if span["span_id"] != "span-1" || span["parent_span_id"] != "root" || span["trace_id"] != "trace-1" {
		t.Fatalf("span aliases = %#v", span)
	}
}

func TestAppsTraceGet_NormalizesKVAttributesToObject(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/trace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"spans": []interface{}{
					map[string]interface{}{
						"trace_id": "trace-1",
						"span_id":  "span-1",
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

	if err := runAppsShortcut(t, AppsTraceGet, []string{"+trace-get", "--app-id", "app_x", "--trace-id", "trace-1", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Spans []map[string]interface{} `json:"spans"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	attrs, ok := env.Data.Spans[0]["attributes"].(map[string]interface{})
	if !ok {
		t.Fatalf("attributes = %#v, want object", env.Data.Spans[0]["attributes"])
	}
	if attrs["app_env"] != "runtime" || attrs["duration_ms"] != "8263" || attrs["module"] != "gateway" {
		t.Fatalf("attributes = %#v", attrs)
	}
}
