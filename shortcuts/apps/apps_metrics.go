// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultAppsMetricEnv          = "online"
	defaultAppsMetricDownSample   = "1m"
	metricListEndpoint            = "query_metrics_data"
	defaultObservabilityRangeDays = 30
)

// AppsMetricList lists online app observability metrics.
var AppsMetricList = common.Shortcut{
	Service:     appsService,
	Command:     "+metric-list",
	Description: "List online app request, latency, CPU, and memory metrics",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +metric-list --app-id <app_id> --metric requests --series total --since 1d",
		"Tip: metric timestamps use seconds; use +analytics-list for PV/UV-style analytics.",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID whose online metrics should be listed", Required: true},
		{Name: appsEnvironmentFlag, Default: defaultAppsMetricEnv, Desc: "observability environment; only online is supported"},
		{Name: "metric", Desc: "metric family to list", Required: true, Enum: []string{"requests", "latency", "cpu", "memory"}},
		{Name: "series", Desc: "metric series within the family, such as total/error or p50/p99"},
		{Name: "since", Desc: "start time, relative duration (30s, 5m, 0.5h, 2h, 3d, 1w), local date/time, or RFC3339; defaults to 30 days before --until"},
		{Name: "until", Desc: "end time, relative duration (30s, 5m, 0.5h, 2h, 3d, 1w), local date/time, or RFC3339; defaults to now"},
		{Name: "page", Type: "string_array", Desc: "frontend page or route filter; repeatable"},
		{Name: "api", Type: "string_array", Desc: "API path/name filter; repeatable"},
		{Name: "down-sample", Default: defaultAppsMetricDownSample, Desc: "metric down-sample interval", Enum: []string{"1m", "1h", "1d"}},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		_, _, _, _, err := buildMetricListBody(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _, _, _, _ := buildMetricListBody(rctx)
		return common.NewDryRunAPI().
			POST(metricListPath(rctx.Str("app-id"))).
			Desc("List online app metrics").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, _ := requireAppID(rctx.Str("app-id"))
		body, names, labels, fillZero, err := buildMetricListBody(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", metricListPath(appID), nil, body)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := observabilitySeriesOutput{
			Items:   normalizeMetricSeries(data, names, labels, fillZero),
			HasMore: false,
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			rows := observabilitySeriesRows(out.Items)
			sortObservabilityRowsDesc(rows, "timestamp")
			rows = filterObservabilityRowsWithTime(rows, "timestamp")
			appsPrintSchemaTable(w, rows, metricSeriesSchema(labels, strings.TrimSpace(strings.ToLower(rctx.Str("metric"))) == "latency"))
		})
		return nil
	},
}

type observabilitySeriesOutput struct {
	Items   []map[string]interface{} `json:"items"`
	HasMore bool                     `json:"has_more"`
}

func metricListPath(appID string) string {
	return appScopedPath(appID, metricListEndpoint)
}

func buildMetricListBody(rctx *common.RuntimeContext) (map[string]interface{}, []string, []string, bool, error) {
	env := strings.TrimSpace(rctx.Str(appsEnvironmentFlag))
	if env == "" {
		env = defaultAppsMetricEnv
	}
	if err := validateObservabilityEnv(env); err != nil {
		return nil, nil, nil, false, err
	}
	names, labels, err := metricNamesForCLI(rctx.Str("metric"), rctx.Str("series"))
	if err != nil {
		return nil, nil, nil, false, err
	}
	since, until, err := defaultedObservabilityTimeRange(rctx.Str("since"), rctx.Str("until"))
	if err != nil {
		return nil, nil, nil, false, err
	}
	downSample := strings.TrimSpace(rctx.Str("down-sample"))
	if !rctx.Changed("down-sample") {
		downSample = appsMetricDownSampleForRange(since, until)
	} else if downSample == "" {
		downSample = defaultAppsMetricDownSample
	}
	body := map[string]interface{}{
		"metric_names":         names,
		"start_timestamp":      secNumber(since),
		"end_timestamp":        secNumber(until),
		"down_sample":          downSample,
		"need_pack_lack_point": false,
	}
	if filter := buildMetricListFilter(rctx); len(filter) > 0 {
		body["filter"] = filter
	}
	return body, names, labels, strings.TrimSpace(strings.ToLower(rctx.Str("metric"))) == "requests", nil
}

func appsMetricDownSampleForRange(since, until time.Time) string {
	d := until.Sub(since)
	switch {
	case d <= 6*time.Hour:
		return "1m"
	case d <= 7*24*time.Hour:
		return "1h"
	default:
		return "1d"
	}
}

func buildMetricListFilter(rctx *common.RuntimeContext) map[string]interface{} {
	filter := make(map[string]interface{})
	if pages := cleanRepeatedStrings(rctx.StrArray("page")); len(pages) > 0 {
		filter["pages"] = pages
	}
	if apis := cleanRepeatedStrings(rctx.StrArray("api")); len(apis) > 0 {
		filter["apis"] = apis
	}
	return filter
}

func defaultedObservabilityTimeRange(sinceRaw, untilRaw string) (time.Time, time.Time, error) {
	since, until, hasSince, hasUntil, err := parseAppsTimeRange("--since", sinceRaw, "--until", untilRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !hasUntil {
		until = time.Now()
	}
	if !hasSince {
		since = until.Add(-defaultObservabilityRangeDays * 24 * time.Hour)
	}
	if since.After(until) {
		return time.Time{}, time.Time{}, appsValidationParamError("--until", "--until must be greater than or equal to --since")
	}
	return since, until, nil
}

func metricNamesForCLI(metric, series string) ([]string, []string, error) {
	metric = strings.TrimSpace(strings.ToLower(metric))
	series = strings.TrimSpace(strings.ToLower(series))
	switch metric {
	case "requests":
		switch series {
		case "":
			return []string{"client_api_request_count", "client_api_request_error_count"}, []string{"total", "error"}, nil
		case "total":
			return []string{"client_api_request_count"}, []string{"total"}, nil
		case "error":
			return []string{"client_api_request_error_count"}, []string{"error"}, nil
		default:
			return nil, nil, appsValidationParamError("--series", "--series for --metric requests must be total or error")
		}
	case "latency":
		switch series {
		case "":
			return []string{"client_api_request_latency_p50", "client_api_request_latency_p99"}, []string{"p50", "p99"}, nil
		case "p50":
			return []string{"client_api_request_latency_p50"}, []string{"p50"}, nil
		case "p99":
			return []string{"client_api_request_latency_p99"}, []string{"p99"}, nil
		default:
			return nil, nil, appsValidationParamError("--series", "--series for --metric latency must be p50 or p99")
		}
	case "cpu":
		if series != "" {
			return nil, nil, appsValidationParamError("--series", "--metric cpu does not support --series")
		}
		return []string{"cpu_usage"}, []string{"cpu"}, nil
	case "memory":
		if series != "" {
			return nil, nil, appsValidationParamError("--series", "--metric memory does not support --series")
		}
		return []string{"mem_usage"}, []string{"memory"}, nil
	default:
		return nil, nil, appsValidationParamError("--metric", "--metric must be one of requests, latency, cpu, memory")
	}
}

func normalizeMetricSeries(data map[string]interface{}, names, labels []string, fillZero bool) []map[string]interface{} {
	return normalizeObservabilitySeries(data, labels, observabilityNameLabels(names, labels), fillZero, "timestamp")
}

func normalizeObservabilitySeries(data map[string]interface{}, labels []string, nameLabels map[string]string, fillZero bool, timeField string) []map[string]interface{} {
	if series := observabilityMapSlice(data["series"]); len(series) > 0 {
		return mergeObservabilitySeries(series, labels, nameLabels, fillZero, timeField)
	}
	if items := observabilityMapSlice(data["items"]); len(items) > 0 {
		if observabilityHasNestedPoints(items) {
			return mergeObservabilitySeries(items, labels, nameLabels, fillZero, timeField)
		}
		return normalizeObservabilityPoints(items, labels, nameLabels, fillZero, timeField)
	}
	for _, key := range []string{"points", "data_points", "dataPoints"} {
		if points := observabilityMapSlice(data[key]); len(points) > 0 {
			return normalizeObservabilityPoints(points, labels, nameLabels, fillZero, timeField)
		}
	}
	return []map[string]interface{}{}
}

func observabilityHasNestedPoints(items []map[string]interface{}) bool {
	for _, item := range items {
		if len(observabilityNestedPoints(item)) > 0 {
			return true
		}
	}
	return false
}

func mergeObservabilitySeries(series []map[string]interface{}, labels []string, nameLabels map[string]string, fillZero bool, timeField string) []map[string]interface{} {
	index := make(map[string]int)
	items := make([]map[string]interface{}, 0)
	for i, serie := range series {
		label := observabilitySeriesLabel(serie, labels, nameLabels, i)
		if label == "" {
			continue
		}
		points := observabilityNestedPoints(serie)
		if len(points) == 0 {
			points = []map[string]interface{}{serie}
		}
		for _, point := range points {
			timestamp := observabilityTimestamp(point, timeField)
			dimensions := observabilityDimensions(point)
			key := observabilityPointKey(timestamp, dimensions)
			pos, ok := index[key]
			if !ok {
				pos = len(items)
				index[key] = pos
				items = append(items, map[string]interface{}{
					timeField:    timestamp,
					"dimensions": dimensions,
					"values":     map[string]interface{}{},
				})
			}
			values := items[pos]["values"].(map[string]interface{})
			values[label] = observabilityPointValue(point, label, nameLabels)
		}
	}
	if fillZero {
		fillObservabilityZeroes(items, labels)
	}
	return items
}

func normalizeObservabilityPoints(points []map[string]interface{}, labels []string, nameLabels map[string]string, fillZero bool, timeField string) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(points))
	for _, point := range points {
		values := observabilityPointValues(point, labels, nameLabels, fillZero)
		items = append(items, map[string]interface{}{
			timeField:    observabilityTimestamp(point, timeField),
			"dimensions": observabilityDimensions(point),
			"values":     values,
		})
	}
	return items
}

func fillObservabilityZeroes(items []map[string]interface{}, labels []string) {
	for _, item := range items {
		values, ok := item["values"].(map[string]interface{})
		if !ok {
			values = map[string]interface{}{}
			item["values"] = values
		}
		for _, label := range labels {
			if value, ok := values[label]; !ok || value == nil {
				values[label] = 0
			}
		}
	}
}

func fillObservabilityZeroesWhenPartiallyPresent(items []map[string]interface{}, labels []string) {
	for _, item := range items {
		values, ok := item["values"].(map[string]interface{})
		if !ok || !observabilityHasAnyNonNullValue(values) {
			continue
		}
		for _, label := range labels {
			if value, ok := values[label]; !ok || value == nil {
				values[label] = 0
			}
		}
	}
}

func observabilityHasAnyNonNullValue(values map[string]interface{}) bool {
	for _, value := range values {
		if value != nil {
			return true
		}
	}
	return false
}

func observabilityPointValues(point map[string]interface{}, labels []string, nameLabels map[string]string, fillZero bool) map[string]interface{} {
	values := make(map[string]interface{}, len(labels))
	switch raw := firstObservabilityValue(point, "values", "value_map", "valueMap"); v := raw.(type) {
	case map[string]interface{}:
		for _, label := range labels {
			if value, ok := v[label]; ok {
				values[label] = value
			}
		}
		for name, label := range nameLabels {
			if value, ok := v[name]; ok {
				values[label] = value
			}
		}
	case []interface{}:
		for i, rawItem := range v {
			if item, ok := rawItem.(map[string]interface{}); ok {
				name := strings.TrimSpace(fmt.Sprint(firstObservabilityValue(item, "metric_name", "metricName", "name")))
				label := nameLabels[name]
				if label == "" && i < len(labels) {
					label = labels[i]
				}
				if label != "" {
					values[label] = firstObservabilityValue(item, "value")
				}
				continue
			}
			if i < len(labels) {
				values[labels[i]] = rawItem
			}
		}
	}
	for _, label := range labels {
		if value, ok := point[label]; ok {
			values[label] = value
		}
	}
	if len(labels) == 1 {
		if value, ok := point["value"]; ok {
			values[labels[0]] = value
		}
	}
	if fillZero {
		for _, label := range labels {
			if value, ok := values[label]; !ok || value == nil {
				values[label] = 0
			}
		}
	}
	return values
}

func observabilityPointValue(point map[string]interface{}, label string, nameLabels map[string]string) interface{} {
	if value, ok := point["value"]; ok {
		return value
	}
	switch raw := firstObservabilityValue(point, "values", "value_map", "valueMap"); values := raw.(type) {
	case map[string]interface{}:
		for name, mappedLabel := range nameLabels {
			if mappedLabel == label {
				if value, ok := values[name]; ok {
					return value
				}
			}
		}
		return values[label]
	case []interface{}:
		for _, rawItem := range values {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				continue
			}
			name := strings.TrimSpace(fmt.Sprint(firstObservabilityValue(item, "metric_name", "metricName", "name")))
			if nameLabels[name] == label {
				return firstObservabilityValue(item, "value")
			}
		}
		for _, rawItem := range values {
			if _, ok := rawItem.(map[string]interface{}); !ok {
				return rawItem
			}
		}
	}
	return nil
}

func observabilityNestedPoints(item map[string]interface{}) []map[string]interface{} {
	for _, key := range []string{"data_points", "dataPoints", "points", "items"} {
		if points := observabilityMapSlice(item[key]); len(points) > 0 {
			return points
		}
	}
	return nil
}

func observabilityMapSlice(raw interface{}) []map[string]interface{} {
	switch items := raw.(type) {
	case []map[string]interface{}:
		return items
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func observabilitySeriesLabel(serie map[string]interface{}, labels []string, nameLabels map[string]string, index int) string {
	for _, key := range []string{"label", "series", "name", "metric_name", "metricName", "metric_type", "metricType"} {
		if value, ok := serie[key].(string); ok {
			value = strings.TrimSpace(value)
			if label := nameLabels[value]; label != "" {
				return label
			}
			if containsObservabilityLabel(labels, value) {
				return value
			}
		}
	}
	if index >= 0 && index < len(labels) {
		return labels[index]
	}
	return ""
}

func containsObservabilityLabel(labels []string, value string) bool {
	for _, label := range labels {
		if value == label {
			return true
		}
	}
	return false
}

func observabilityTimestamp(point map[string]interface{}, timeField string) interface{} {
	keys := []string{timeField}
	if timeField == "timestamp_ns" {
		keys = append(keys, "timestampNs", "time_ns", "timeNs", "time", "ts")
	} else {
		keys = append(keys, "timestampSec", "time", "ts")
	}
	return firstObservabilityValue(point, keys...)
}

func observabilityDimensions(point map[string]interface{}) map[string]interface{} {
	for _, key := range []string{"dimensions", "dimension", "labels", "tags"} {
		if dimensions, ok := point[key].(map[string]interface{}); ok {
			return cloneMap(dimensions)
		}
		if dimensions := observabilityKVList(point[key]); len(dimensions) > 0 {
			return dimensions
		}
	}
	return map[string]interface{}{}
}

func observabilityNameLabels(names, labels []string) map[string]string {
	out := make(map[string]string, len(names))
	for i, name := range names {
		if i < len(labels) {
			out[name] = labels[i]
		}
	}
	return out
}

func observabilityKVList(raw interface{}) map[string]interface{} {
	items := observabilityMapSlice(raw)
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(items))
	for _, item := range items {
		key := strings.TrimSpace(fmt.Sprint(firstObservabilityValue(item, "key", "name")))
		if key == "" {
			continue
		}
		out[key] = firstObservabilityValue(item, "value")
	}
	return out
}

func firstObservabilityValue(m map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			return value
		}
	}
	return nil
}

func observabilityPointKey(timestamp interface{}, dimensions map[string]interface{}) string {
	encoded, err := json.Marshal(dimensions)
	if err != nil {
		return fmt.Sprintf("%v|%v", timestamp, dimensions)
	}
	return fmt.Sprintf("%v|%s", timestamp, string(encoded))
}

func observabilitySeriesRows(items []map[string]interface{}) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		row := map[string]interface{}{}
		for key, value := range item {
			if key == "values" {
				if values, ok := value.(map[string]interface{}); ok {
					for label, metricValue := range values {
						row[label] = metricValue
					}
				}
				continue
			}
			row[key] = value
		}
		rows = append(rows, row)
	}
	return rows
}

func metricSeriesSchema(labels []string, durationValues bool) appsOutputSchema {
	columns := []appsOutputColumn{
		{Key: "timestamp", Label: "time", Format: appsFormatSec("2006-01-02 15:04:05")},
	}
	for _, label := range labels {
		col := appsOutputColumn{Key: label}
		if durationValues {
			col.Format = appsFormatDurationMS
		}
		columns = append(columns, col)
	}
	return appsOutputSchema{Columns: columns, Strict: true}
}

func sortObservabilityRowsDesc(rows []map[string]interface{}, key string) {
	sort.SliceStable(rows, func(i, j int) bool {
		left, leftOK := appsInt64Value(rows[i][key])
		right, rightOK := appsInt64Value(rows[j][key])
		if !leftOK || !rightOK {
			return false
		}
		return left > right
	})
}

func filterObservabilityRowsWithTime(rows []map[string]interface{}, key string) []map[string]interface{} {
	out := rows[:0]
	for _, row := range rows {
		if _, ok := appsInt64Value(row[key]); ok {
			out = append(out, row)
		}
	}
	return out
}
