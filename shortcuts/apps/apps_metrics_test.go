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

func TestAppsMetricList_DryRunUsesSeconds(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsMetricList, []string{
		"+metric-list", "--app-id", "app_x", "--metric", "requests",
		"--series", "total", "--since", "2026-06-23T10:00:00Z",
		"--until", "2026-06-23T10:01:00Z", "--down-sample", "1m",
		"--dry-run", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
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

func TestAppsMetricList_AutoDownSampleByRange(t *testing.T) {
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
			err := runAppsShortcut(t, AppsMetricList, []string{
				"+metric-list", "--app-id", "app_x", "--metric", "requests",
				"--since", tc.since, "--until", tc.until, "--dry-run", "--as", "user",
			}, factory, stdout)
			if err != nil {
				t.Fatalf("dry-run err=%v", err)
			}
			var env dryRunAPIEnvelope
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
			}
			if got := env.API[0].Body["down_sample"]; got != tc.want {
				t.Fatalf("down_sample = %#v, want %q; stdout:\n%s", got, tc.want, stdout.String())
			}
		})
	}
}

func TestAppsMetricList_RejectsDevEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsMetricList, []string{
		"+metric-list", "--app-id", "app_x", "--metric", "requests", "--environment", "dev", "--as", "user",
	}, factory, stdout)
	requireAppsValidationParam(t, err, "--environment")
}

func TestAppsMetricList_FillsMissingRequestValuesWithZero(t *testing.T) {
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

	if err := runAppsShortcut(t, AppsMetricList, []string{
		"+metric-list", "--app-id", "app_x", "--metric", "requests", "--as", "user",
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

func TestAppsMetricList_PrettyFormatsTimeFirst(t *testing.T) {
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

	if err := runAppsShortcut(t, AppsMetricList, []string{
		"+metric-list", "--app-id", "app_x", "--metric", "requests", "--format", "pretty", "--as", "user",
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

func TestAppsMetricList_NamedSeriesDoesNotDependOnBackendOrder(t *testing.T) {
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

	if err := runAppsShortcut(t, AppsMetricList, []string{
		"+metric-list", "--app-id", "app_x", "--metric", "requests", "--as", "user",
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

func TestAppsMetricList_EmptyResponseOutputsEmptyItemsArray(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/query_metrics_data",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	if err := runAppsShortcut(t, AppsMetricList, []string{
		"+metric-list", "--app-id", "app_x", "--metric", "latency", "--as", "user",
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
