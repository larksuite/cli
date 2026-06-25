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

func TestMetricNamesMapping(t *testing.T) {
	got, labels, err := metricNamesForCLI("requests", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "client_api_request_count,client_api_request_error_count" {
		t.Fatalf("names = %#v", got)
	}
	if strings.Join(labels, ",") != "total,error" {
		t.Fatalf("labels = %#v", labels)
	}
	if _, _, err := metricNamesForCLI("cpu", "p99"); err == nil {
		t.Fatalf("cpu with p99 should fail")
	}
}

func TestAppsMetricQuery_DryRunUsesSeconds(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsMetricQuery, []string{
		"+metric-query", "--app-id", "app_x", "--metric", "requests",
		"--series", "total", "--since", "2026-06-23T10:00:00Z",
		"--until", "2026-06-23T10:01:00Z", "--down-sample", "1m",
		"--dry-run", "--as", "user",
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
	if env.API[0].Method != "POST" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/query_metrics_data" {
		t.Fatalf("method/url = %s %s", env.API[0].Method, env.API[0].URL)
	}
	body := env.API[0].Body
	if _, ok := body["start_timestamp"]; !ok {
		t.Fatalf("metric dry-run missing start_timestamp: %#v", body)
	}
	if _, ok := body["start_timestamp_ns"]; ok {
		t.Fatalf("metric should not use start_timestamp_ns: %#v", body)
	}
	if _, ok := body["app_env"]; ok {
		t.Fatalf("metric OpenAPI body should not include app_env: %#v", body)
	}
	if body["start_timestamp"] != "1782208800" || body["end_timestamp"] != "1782208860" {
		t.Fatalf("metric timestamps = %v %v", body["start_timestamp"], body["end_timestamp"])
	}
	if body["down_sample"] != "1m" {
		t.Fatalf("down_sample = %v", body["down_sample"])
	}
}

func TestAppsMetricQuery_AutoDownSampleByRange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		since string
		until string
		want  string
	}{
		{name: "short", since: "2026-06-23T10:00:00Z", until: "2026-06-23T12:00:00Z", want: "1m"},
		{name: "medium", since: "2026-06-21T10:00:00Z", until: "2026-06-23T10:00:00Z", want: "1h"},
		{name: "long", since: "2026-06-01T10:00:00Z", until: "2026-06-23T10:00:00Z", want: "1d"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsMetricQuery, []string{
				"+metric-query", "--app-id", "app_x", "--metric", "requests",
				"--since", tc.since, "--until", tc.until, "--dry-run", "--as", "user",
			}, factory, stdout)
			if err != nil {
				t.Fatalf("dry-run err=%v", err)
			}
			var env struct {
				API []struct {
					Body map[string]interface{} `json:"body"`
				} `json:"api"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
			}
			if got := env.API[0].Body["down_sample"]; got != tc.want {
				t.Fatalf("down_sample = %#v, want %q; stdout:\n%s", got, tc.want, stdout.String())
			}
		})
	}
}

func TestAppsMetricQuery_RejectsDevEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsMetricQuery, []string{
		"+metric-query", "--app-id", "app_x", "--metric", "requests", "--environment", "dev", "--as", "user",
	}, factory, stdout)
	requireAppsValidationParam(t, err, "--environment")
}

func TestAppsMetricQuery_FillsMissingRequestValuesWithZero(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_metrics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"points": []interface{}{
					map[string]interface{}{
						"timestamp":  float64(1782208800),
						"dimensions": map[string]interface{}{"page": "/home"},
						"values": []interface{}{
							map[string]interface{}{"metric_name": "client_api_request_count", "value": float64(12)},
						},
					},
					map[string]interface{}{
						"timestamp":  float64(1782208860),
						"dimensions": map[string]interface{}{"page": "/settings"},
						"values": []interface{}{
							map[string]interface{}{"metric_name": "client_api_request_count", "value": float64(8)},
							map[string]interface{}{"metric_name": "client_api_request_error_count", "value": nil},
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsMetricQuery, []string{
		"+metric-query", "--app-id", "app_x", "--metric", "requests", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items []struct {
				Values map[string]interface{} `json:"values"`
			} `json:"items"`
			HasMore bool `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if env.Data.HasMore {
		t.Fatalf("has_more = true, want false")
	}
	if len(env.Data.Items) != 2 {
		t.Fatalf("items len = %d", len(env.Data.Items))
	}
	for i, item := range env.Data.Items {
		if item.Values["error"] != float64(0) {
			t.Fatalf("item %d error = %#v, want 0; values=%#v", i, item.Values["error"], item.Values)
		}
	}
}

func TestAppsMetricQuery_PrettyFormatsTimeFirst(t *testing.T) {
	const rawSec = int64(1782208800)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_metrics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"points": []interface{}{
					map[string]interface{}{
						"timestamp": float64(rawSec),
						"values": []interface{}{
							map[string]interface{}{"metric_name": "client_api_request_count", "value": float64(12)},
							map[string]interface{}{"metric_name": "client_api_request_error_count", "value": float64(1)},
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsMetricQuery, []string{
		"+metric-query", "--app-id", "app_x", "--metric", "requests", "--format", "pretty", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	wantTime := time.Unix(rawSec, 0).Local().Format("2006-01-02 15:04:05")
	if !strings.HasPrefix(got, "time") {
		t.Fatalf("pretty output should start with time column, got:\n%s", got)
	}
	if !strings.Contains(got, wantTime) {
		t.Fatalf("pretty output missing formatted time %q:\n%s", wantTime, got)
	}
	if strings.Contains(got, "timestamp") || strings.Contains(got, "1782208800") {
		t.Fatalf("pretty output should hide raw timestamp, got:\n%s", got)
	}
}

func TestAppsMetricQuery_NamedSeriesDoesNotDependOnBackendOrder(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_metrics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"series": []interface{}{
					map[string]interface{}{
						"name": "client_api_request_error_count",
						"points": []interface{}{
							map[string]interface{}{"timestamp": float64(1782208800), "value": float64(2)},
						},
					},
					map[string]interface{}{
						"name": "client_api_request_count",
						"points": []interface{}{
							map[string]interface{}{"timestamp": float64(1782208800), "value": float64(10)},
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsMetricQuery, []string{
		"+metric-query", "--app-id", "app_x", "--metric", "requests", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items []struct {
				Values map[string]interface{} `json:"values"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if len(env.Data.Items) != 1 {
		t.Fatalf("items len = %d", len(env.Data.Items))
	}
	values := env.Data.Items[0].Values
	if values["total"] != float64(10) || values["error"] != float64(2) {
		t.Fatalf("values = %#v, want total=10 error=2", values)
	}
}

func TestAppsMetricQuery_EmptyResponseOutputsEmptyItemsArray(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_metrics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	if err := runAppsShortcut(t, AppsMetricQuery, []string{
		"+metric-query", "--app-id", "app_x", "--metric", "latency", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items   []map[string]interface{} `json:"items"`
			HasMore bool                     `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if env.Data.Items == nil {
		t.Fatalf("items decoded as nil; stdout=%s", stdout.String())
	}
	if len(env.Data.Items) != 0 || env.Data.HasMore {
		t.Fatalf("empty output = items %#v has_more %v", env.Data.Items, env.Data.HasMore)
	}
}

func TestAppsAnalyticsQuery_DryRunUsesNanoseconds(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAnalyticsQuery, []string{
		"+analytics-query", "--app-id", "app_x", "--analytics", "users",
		"--since", "2026-06-23T10:00:00Z", "--until", "2026-06-23T10:01:00Z",
		"--granularity", "week", "--dry-run", "--as", "user",
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
	if env.API[0].Method != "POST" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/query_analytics_data" {
		t.Fatalf("method/url = %s %s", env.API[0].Method, env.API[0].URL)
	}
	body := env.API[0].Body
	if _, ok := body["start_timestamp_ns"]; !ok {
		t.Fatalf("analytics dry-run missing start_timestamp_ns: %#v", body)
	}
	if _, ok := body["start_timestamp"]; ok {
		t.Fatalf("analytics should not use start_timestamp: %#v", body)
	}
	if body["time_aggregation_unit"] != "WEEK" {
		t.Fatalf("time_aggregation_unit = %v", body["time_aggregation_unit"])
	}
	if _, ok := body["app_env"]; ok {
		t.Fatalf("analytics OpenAPI body should not include app_env: %#v", body)
	}
	if _, ok := body["analytics_types"]; ok {
		t.Fatalf("analytics OpenAPI body should use metric_types, not analytics_types: %#v", body)
	}
	if body["need_pack_lack_point"] != false {
		t.Fatalf("need_pack_lack_point = %#v, want false", body["need_pack_lack_point"])
	}
	if _, ok := body["group_by"]; ok {
		t.Fatalf("group_by is intentionally unsupported for now: %#v", body)
	}
	if metricTypes, ok := body["metric_types"].([]interface{}); !ok || len(metricTypes) != 3 {
		t.Fatalf("metric_types = %#v", body["metric_types"])
	}
	if body["start_timestamp_ns"] != "1782208800000000000" ||
		body["end_timestamp_ns"] != "1782208860000000000" {
		t.Fatalf("analytics timestamps = %#v %#v", body["start_timestamp_ns"], body["end_timestamp_ns"])
	}
}

func TestAppsAnalyticsQuery_PageViewDesktopSeriesSetsDeviceFilter(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "series",
			args: []string{
				"+analytics-query", "--app-id", "app_x", "--analytics", "page-view",
				"--series", "desktop", "--page", "/home", "--dry-run", "--as", "user",
			},
		},
		{
			name: "device-type",
			args: []string{
				"+analytics-query", "--app-id", "app_x", "--analytics", "page-view",
				"--device-type", "desktop", "--dry-run", "--as", "user",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			if err := runAppsShortcut(t, AppsAnalyticsQuery, tc.args, factory, stdout); err != nil {
				t.Fatalf("dry-run err=%v", err)
			}
			var env struct {
				API []struct {
					Body map[string]interface{} `json:"body"`
				} `json:"api"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
			}
			filter := env.API[0].Body["filter"].(map[string]interface{})
			deviceTypes := filter["device_types"].([]interface{})
			if len(deviceTypes) != 1 || deviceTypes[0] != "desktop" {
				t.Fatalf("device_types = %#v", deviceTypes)
			}
			if tc.name == "series" && filter["page"] != "/home" {
				t.Fatalf("filter.page = %#v, want /home", filter["page"])
			}
		})
	}
}

func TestAppsAnalyticsQuery_DesktopSeriesUsesDesktopValueLabel(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_analytics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"series": []interface{}{
					map[string]interface{}{
						"metric_type": "PAGE_VIEW",
						"points": []interface{}{
							map[string]interface{}{
								"timestamp_ns": float64(1782208800000000000),
								"value":        float64(21),
							},
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsAnalyticsQuery, []string{
		"+analytics-query", "--app-id", "app_x", "--analytics", "page-view",
		"--series", "desktop", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items []struct {
				Values map[string]interface{} `json:"values"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if len(env.Data.Items) != 1 {
		t.Fatalf("items len = %d", len(env.Data.Items))
	}
	if env.Data.Items[0].Values["desktop"] != float64(21) {
		t.Fatalf("values = %#v, want desktop=21", env.Data.Items[0].Values)
	}
	if _, ok := env.Data.Items[0].Values["page-view"]; ok {
		t.Fatalf("values should not use page-view label: %#v", env.Data.Items[0].Values)
	}
}

func TestAppsAnalyticsQuery_PrettyFormatsTimeFirst(t *testing.T) {
	const rawNS = int64(1782208800000000000)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_analytics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"series": []interface{}{
					map[string]interface{}{
						"metric_type": "ACTIVE_USER",
						"points": []interface{}{
							map[string]interface{}{"timestamp_ns": float64(rawNS), "value": float64(7)},
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsAnalyticsQuery, []string{
		"+analytics-query", "--app-id", "app_x", "--analytics", "users", "--series", "active", "--format", "pretty", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	wantTime := time.Unix(0, rawNS).Local().Format("2006-01-02 15:04:05")
	if !strings.HasPrefix(got, "time") {
		t.Fatalf("pretty output should start with time column, got:\n%s", got)
	}
	if !strings.Contains(got, wantTime) {
		t.Fatalf("pretty output missing formatted time %q:\n%s", wantTime, got)
	}
	if strings.Contains(got, "timestamp_ns") || strings.Contains(got, "1782208800000000000") {
		t.Fatalf("pretty output should hide raw timestamp_ns, got:\n%s", got)
	}
}

func TestAppsAnalyticsQuery_PrettySkipsRowsWithoutTime(t *testing.T) {
	const rawNS = int64(1782208800000000000)
	rows := []map[string]interface{}{
		{"timestamp_ns": rawNS, "active-users": float64(7)},
		{"active-users": float64(0)},
	}
	sortObservabilityRowsDesc(rows, "timestamp_ns")
	rows = filterObservabilityRowsWithTime(rows, "timestamp_ns")
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1: %#v", len(rows), rows)
	}
	if rows[0]["timestamp_ns"] != rawNS {
		t.Fatalf("remaining row = %#v", rows[0])
	}
}

func TestAppsAnalyticsQuery_NamedSeriesDoesNotDependOnBackendOrder(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_analytics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"series": []interface{}{
					map[string]interface{}{
						"metric_type": "TOTAL_USER",
						"points": []interface{}{
							map[string]interface{}{"timestamp_ns": float64(1782208800000000000), "value": float64(20)},
						},
					},
					map[string]interface{}{
						"metric_type": "ACTIVE_USER",
						"points": []interface{}{
							map[string]interface{}{"timestamp_ns": float64(1782208800000000000), "value": float64(7)},
						},
					},
					map[string]interface{}{
						"metric_type": "NEW_USER",
						"points": []interface{}{
							map[string]interface{}{"timestamp_ns": float64(1782208800000000000), "value": float64(3)},
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsAnalyticsQuery, []string{
		"+analytics-query", "--app-id", "app_x", "--analytics", "users", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items []struct {
				Values map[string]interface{} `json:"values"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if len(env.Data.Items) != 1 {
		t.Fatalf("items len = %d", len(env.Data.Items))
	}
	values := env.Data.Items[0].Values
	if values["active-users"] != float64(7) || values["new-users"] != float64(3) || values["total-users"] != float64(20) {
		t.Fatalf("values = %#v, want active-users=7 new-users=3 total-users=20", values)
	}
}

func TestAppsAnalyticsQuery_FillsMissingAndNullValuesWhenAnyValuePresent(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_analytics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"timestamp_ns": "1782208800000000000",
						"values": map[string]interface{}{
							"total-users":  float64(4),
							"active-users": nil,
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsAnalyticsQuery, []string{
		"+analytics-query", "--app-id", "app_x", "--analytics", "users", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items []struct {
				Values map[string]interface{} `json:"values"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	values := env.Data.Items[0].Values
	if values["total-users"] != float64(4) || values["active-users"] != float64(0) || values["new-users"] != float64(0) {
		t.Fatalf("values = %#v, want total-users=4 active-users=0 new-users=0", values)
	}
}

func TestAppsAnalyticsQuery_DoesNotFillAllNullValues(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_analytics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"timestamp_ns": "1782208800000000000",
						"values": map[string]interface{}{
							"total-users":  nil,
							"active-users": nil,
						},
					},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsAnalyticsQuery, []string{
		"+analytics-query", "--app-id", "app_x", "--analytics", "users", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items []struct {
				Values map[string]interface{} `json:"values"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	values := env.Data.Items[0].Values
	if values["total-users"] != nil || values["active-users"] != nil {
		t.Fatalf("values = %#v, want existing nulls preserved", values)
	}
	if _, ok := values["new-users"]; ok {
		t.Fatalf("values should not fill missing labels when all present values are null: %#v", values)
	}
}

func TestAppsAnalyticsQuery_EmptyResponseOutputsEmptyItemsArray(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_analytics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	if err := runAppsShortcut(t, AppsAnalyticsQuery, []string{
		"+analytics-query", "--app-id", "app_x", "--analytics", "users", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var env struct {
		Data struct {
			Items   []map[string]interface{} `json:"items"`
			HasMore bool                     `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if env.Data.Items == nil {
		t.Fatalf("items decoded as nil; stdout=%s", stdout.String())
	}
	if len(env.Data.Items) != 0 || env.Data.HasMore {
		t.Fatalf("empty output = items %#v has_more %v", env.Data.Items, env.Data.HasMore)
	}
}

func TestAnalyticsTypesMapping(t *testing.T) {
	types, labels, filter, err := analyticsTypesForCLI("users", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != "ACTIVE_USER,NEW_USER,TOTAL_USER" {
		t.Fatalf("types = %#v", types)
	}
	if strings.Join(labels, ",") != "active-users,new-users,total-users" {
		t.Fatalf("labels = %#v", labels)
	}
	if len(filter) != 0 {
		t.Fatalf("filter = %#v, want empty", filter)
	}

	types, labels, filter, err = analyticsTypesForCLI("page-view", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != "PAGE_VIEW" || strings.Join(labels, ",") != "all" {
		t.Fatalf("page-view all mapping = %#v %#v", types, labels)
	}
	if len(filter) != 0 {
		t.Fatalf("filter = %#v, want empty", filter)
	}

	types, labels, filter, err = analyticsTypesForCLI("page-view", "desktop", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != "PAGE_VIEW" || strings.Join(labels, ",") != "desktop" {
		t.Fatalf("page-view mapping = %#v %#v", types, labels)
	}
	deviceTypes := filter["device_types"].([]string)
	if len(deviceTypes) != 1 || deviceTypes[0] != "desktop" {
		t.Fatalf("device_types = %#v", deviceTypes)
	}

	types, labels, filter, err = analyticsTypesForCLI("page-view", "mobile-view", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != "PAGE_VIEW" || strings.Join(labels, ",") != "mobile" {
		t.Fatalf("page-view mobile mapping = %#v %#v", types, labels)
	}
	deviceTypes = filter["device_types"].([]string)
	if len(deviceTypes) != 1 || deviceTypes[0] != "mobile" {
		t.Fatalf("device_types = %#v", deviceTypes)
	}

	if _, _, _, err := analyticsTypesForCLI("users", "desktop", ""); err == nil {
		t.Fatalf("users desktop series should fail")
	}
	if _, _, _, err := analyticsTypesForCLI("page-view", "tablet", ""); err == nil {
		t.Fatalf("page-view tablet series should fail")
	}
	if _, _, _, err := analyticsTypesForCLI("page-view", "", "tablet"); err == nil {
		t.Fatalf("tablet device type should fail")
	}
}
