// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultAppsTraceEnv = "online"
	traceSearchEndpoint = "search_traces"
	traceGetEndpoint    = "trace"
)

// AppsTraceList searches online app traces with observability filters.
var AppsTraceList = common.Shortcut{
	Service:     appsService,
	Command:     "+trace-list",
	Description: "Search online app traces with observability filters",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +trace-list --app-id <app_id> --trace-id <trace_id>",
		"Tip: use --page-token from the response to fetch the next page.",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID whose online traces should be searched", Required: true},
		{Name: appsEnvironmentFlag, Default: defaultAppsTraceEnv, Desc: "observability environment; only online is supported"},
		{Name: "trace-id", Type: "string_array", Desc: "trace ID filter; repeatable"},
		{Name: "root-span", Desc: "root span keyword filter applied by the trace search backend"},
		{Name: "user-id", Desc: "end user ID filter"},
		{Name: "since", Desc: "start time, relative duration (30s, 5m, 0.5h, 2h, 3d, 1w), local date/time, or RFC3339"},
		{Name: "until", Desc: "end time, relative duration (30s, 5m, 0.5h, 2h, 3d, 1w), local date/time, or RFC3339"},
		{Name: "page-size", Type: "int", Default: fmt.Sprintf("%d", defaultAppsPageSize), Desc: "page size, 1..100"},
		{Name: "page-token", Desc: "pagination cursor from a previous trace search response"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		_, err := buildTraceSearchBody(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildTraceSearchBody(rctx)
		return common.NewDryRunAPI().
			POST(traceSearchPath(rctx.Str("app-id"))).
			Desc("Search online app traces").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, _ := requireAppID(rctx.Str("app-id"))
		body, err := buildTraceSearchBody(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", traceSearchPath(appID), nil, body)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := normalizeTraceSearchResponse(data)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			appsPrintSchemaTable(w, appsProjectRows(traceListRows(out.Items), traceSummarySchema), traceSummarySchema)
		})
		return nil
	},
}

// AppsTraceGet fetches one online app trace by trace ID.
var AppsTraceGet = common.Shortcut{
	Service:     appsService,
	Command:     "+trace-get",
	Description: "Get one online app trace by trace ID",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +trace-get --app-id <app_id> --trace-id <trace_id>",
		"Tip: use +trace-list first if the trace ID is unknown.",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID whose online trace should be fetched", Required: true},
		{Name: appsEnvironmentFlag, Default: defaultAppsTraceEnv, Desc: "observability environment; only online is supported"},
		{Name: "trace-id", Desc: "trace ID to fetch", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("trace-id")) == "" {
			return appsValidationParamError("--trace-id", "--trace-id is required")
		}
		return validateObservabilityEnv(rctx.Str(appsEnvironmentFlag))
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST(traceGetPath(rctx.Str("app-id"))).
			Desc("Get online app trace by trace ID").
			Body(buildTraceGetBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, _ := requireAppID(rctx.Str("app-id"))
		data, err := rctx.CallAPITyped("POST", traceGetPath(appID), nil, buildTraceGetBody(rctx))
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		trace := normalizeTraceDetail(data)
		rctx.OutFormat(trace, nil, func(w io.Writer) {
			appsPrintSchemaTable(w, appsProjectRows([]map[string]interface{}{traceDetailSummary(trace)}, traceSummarySchema), traceSummarySchema)
		})
		return nil
	},
}

type traceSearchOutput struct {
	Items     []map[string]interface{} `json:"items"`
	PageToken string                   `json:"page_token,omitempty"`
	HasMore   bool                     `json:"has_more"`
}

func traceSearchPath(appID string) string {
	return appScopedPath(appID, traceSearchEndpoint)
}

func traceGetPath(appID string) string {
	return appScopedPath(appID, traceGetEndpoint)
}

func buildTraceSearchBody(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	env := strings.TrimSpace(rctx.Str(appsEnvironmentFlag))
	if env == "" {
		env = defaultAppsTraceEnv
	}
	if err := validateObservabilityEnv(env); err != nil {
		return nil, err
	}
	if err := validateAppsPageSize(rctx.Int("page-size")); err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"app_env": appsObservabilityBackendEnv,
		"limit":   rctx.Int("page-size"),
	}
	if token := strings.TrimSpace(rctx.Str("page-token")); token != "" {
		body["page_token"] = token
	}
	if err := addTraceSearchTimeRange(body, rctx); err != nil {
		return nil, err
	}
	if filter := buildTraceSearchFilter(rctx); len(filter) > 0 {
		body["filter"] = filter
	}
	return body, nil
}

func buildTraceGetBody(rctx *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"app_env":  appsObservabilityBackendEnv,
		"trace_id": strings.TrimSpace(rctx.Str("trace-id")),
	}
}

func addTraceSearchTimeRange(body map[string]interface{}, rctx *common.RuntimeContext) error {
	since, until, hasSince, hasUntil, err := parseAppsTimeRange("--since", rctx.Str("since"), "--until", rctx.Str("until"))
	if err != nil {
		return err
	}
	if hasSince {
		body["start_timestamp_ns"] = nsNumber(since)
	}
	if hasUntil {
		body["end_timestamp_ns"] = nsNumber(until)
	}
	return nil
}

func buildTraceSearchFilter(rctx *common.RuntimeContext) map[string]interface{} {
	filter := make(map[string]interface{})
	if traceIDs := cleanRepeatedStrings(rctx.StrArray("trace-id")); len(traceIDs) > 0 {
		filter["trace_ids"] = traceIDs
	}
	addTrimmedTraceFilterString(filter, "keyword", rctx.Str("root-span"))
	addTrimmedTraceFilterStrings(filter, "user_ids", rctx.Str("user-id"))
	return filter
}

func addTrimmedTraceFilterString(filter map[string]interface{}, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		filter[key] = value
	}
}

func addTrimmedTraceFilterStrings(filter map[string]interface{}, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		filter[key] = []string{value}
	}
}

func normalizeTraceSearchResponse(data map[string]interface{}) traceSearchOutput {
	items, sourceKey := firstTraceMapSliceWithKey(data, "items", "trace_items", "traceItems", "spans", "span_items", "spanItems")
	normalized := normalizeTraceSummaries(items)
	if isTraceSpanItemsKey(sourceKey) {
		normalized = aggregateTraceSpanSummaries(items)
	}
	return traceSearchOutput{
		Items:     normalized,
		PageToken: firstLogString(data, "page_token", "next_page_token", "pageToken", "nextPageToken"),
		HasMore:   firstLogBool(data, "has_more", "hasMore"),
	}
}

func firstTraceMapSliceWithKey(data map[string]interface{}, keys ...string) ([]map[string]interface{}, string) {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok {
			continue
		}
		return traceMapSlice(raw), key
	}
	return nil, ""
}

func traceMapSlice(raw interface{}) []map[string]interface{} {
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

func isTraceSpanItemsKey(key string) bool {
	switch key {
	case "spans", "span_items", "spanItems":
		return true
	default:
		return false
	}
}

func normalizeTraceSummaries(items []map[string]interface{}) []map[string]interface{} {
	if len(items) == 0 {
		return nil
	}
	if hasRepeatedTraceID(items) {
		return aggregateTraceSpanSummaries(items)
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeTraceSummary(item))
	}
	return out
}

func hasRepeatedTraceID(items []map[string]interface{}) bool {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		traceID := firstTraceString(item, "trace_id", "traceID", "traceId")
		if traceID == "" {
			continue
		}
		if _, ok := seen[traceID]; ok {
			return true
		}
		seen[traceID] = struct{}{}
	}
	return false
}

func normalizeTraceSummary(item map[string]interface{}) map[string]interface{} {
	out := cloneMap(item)
	copyFirstAlias(out, item, "trace_id", "trace_id", "traceID", "traceId")
	copyFirstAlias(out, item, "start_time_ns", "start_time_ns", "startTimeNs")
	copyFirstAlias(out, item, "root_span", "root_span", "rootSpan")
	copyFirstAlias(out, item, "user_id", "user_id", "userID", "userId")
	copyFirstAlias(out, item, "duration_ms", "duration_ms", "durationMs")
	copyFirstAlias(out, item, "status", "status")
	copyFirstAlias(out, item, "span_count", "span_count", "spanCount")
	return out
}

func aggregateTraceSpanSummaries(spans []map[string]interface{}) []map[string]interface{} {
	groups := make([]traceSpanGroup, 0, len(spans))
	indexByTraceID := make(map[string]int, len(spans))
	ungrouped := make([]map[string]interface{}, 0)
	for _, span := range spans {
		span = normalizeTraceSpan(span)
		traceID := firstTraceString(span, "trace_id", "traceID", "traceId")
		if traceID == "" {
			ungrouped = append(ungrouped, normalizeTraceSummary(span))
			continue
		}
		idx, ok := indexByTraceID[traceID]
		if !ok {
			indexByTraceID[traceID] = len(groups)
			groups = append(groups, traceSpanGroup{traceID: traceID, spans: []map[string]interface{}{span}})
			continue
		}
		groups[idx].spans = append(groups[idx].spans, span)
	}
	out := make([]map[string]interface{}, 0, len(groups)+len(ungrouped))
	for _, group := range groups {
		out = append(out, buildTraceSpanSummary(group.traceID, group.spans))
	}
	out = append(out, ungrouped...)
	return out
}

type traceSpanGroup struct {
	traceID string
	spans   []map[string]interface{}
}

func buildTraceSpanSummary(traceID string, spans []map[string]interface{}) map[string]interface{} {
	root := selectTraceRootCandidate(spans)
	summary := normalizeTraceSummary(root)
	summary["trace_id"] = traceID
	summary["span_count"] = len(spans)
	if firstItemString(summary, "root_span") == "" {
		if rootName := firstItemString(root, "name", "span_name", "spanName"); rootName != "" {
			summary["root_span"] = rootName
		} else if fallbackName := firstTraceSpanName(spans); fallbackName != "" {
			summary["root_span"] = fallbackName
		}
	}
	if firstItemString(summary, "user_id") == "" {
		if userID := firstStringInTraceSpans(spans, "user_id", "userID", "userId"); userID != "" {
			summary["user_id"] = userID
		}
	}
	if startValue, ok := earliestTraceSpanValue(spans, "start_time_ns", "startTimeNs"); ok {
		summary["start_time_ns"] = startValue
	}
	if durationValue, ok := maxTraceSpanValue(spans, "duration_ms", "durationMs"); ok {
		summary["duration_ms"] = durationValue
	}
	if status := aggregateTraceSpanStatus(spans); status != "" {
		summary["status"] = status
	}
	return summary
}

func selectTraceRootCandidate(spans []map[string]interface{}) map[string]interface{} {
	for _, span := range spans {
		if firstItemString(span, "root_span", "rootSpan") != "" {
			return span
		}
	}
	for _, span := range spans {
		if isTraceRootParentCandidate(span) {
			return span
		}
	}
	for _, span := range spans {
		if firstItemString(span, "name", "span_name", "spanName") != "" {
			return span
		}
	}
	if len(spans) == 0 {
		return map[string]interface{}{}
	}
	return spans[0]
}

func isTraceRootParentCandidate(span map[string]interface{}) bool {
	parent, ok := firstTraceValue(span, "parent_span_id", "parentSpanID", "parentSpanId")
	if !ok || parent == nil {
		return true
	}
	parentID, ok := parent.(string)
	return ok && strings.TrimSpace(parentID) == ""
}

func firstTraceSpanName(spans []map[string]interface{}) string {
	return firstStringInTraceSpans(spans, "name", "span_name", "spanName")
}

func firstStringInTraceSpans(spans []map[string]interface{}, keys ...string) string {
	for _, span := range spans {
		if value := firstItemString(span, keys...); value != "" {
			return value
		}
	}
	return ""
}

func earliestTraceSpanValue(spans []map[string]interface{}, keys ...string) (interface{}, bool) {
	var bestValue interface{}
	var bestNumber traceNumber
	var found bool
	for _, span := range spans {
		value, number, ok := firstTraceNumericValue(span, keys...)
		if !ok {
			continue
		}
		if !found || number.less(bestNumber) {
			bestValue = value
			bestNumber = number
			found = true
		}
	}
	return bestValue, found
}

func maxTraceSpanValue(spans []map[string]interface{}, keys ...string) (interface{}, bool) {
	var bestValue interface{}
	var bestNumber traceNumber
	var found bool
	for _, span := range spans {
		value, number, ok := firstTraceNumericValue(span, keys...)
		if !ok {
			continue
		}
		if !found || number.greater(bestNumber) {
			bestValue = value
			bestNumber = number
			found = true
		}
	}
	return bestValue, found
}

func firstTraceNumericValue(span map[string]interface{}, keys ...string) (interface{}, traceNumber, bool) {
	value, ok := firstTraceValue(span, keys...)
	if !ok {
		return nil, traceNumber{}, false
	}
	number, ok := parseTraceNumber(value)
	return value, number, ok
}

type traceNumber struct {
	floatValue float64
	intValue   int64
	exactInt   bool
}

func (n traceNumber) less(other traceNumber) bool {
	if n.exactInt && other.exactInt {
		return n.intValue < other.intValue
	}
	return n.floatValue < other.floatValue
}

func (n traceNumber) greater(other traceNumber) bool {
	if n.exactInt && other.exactInt {
		return n.intValue > other.intValue
	}
	return n.floatValue > other.floatValue
}

func parseTraceNumber(value interface{}) (traceNumber, bool) {
	switch v := value.(type) {
	case int:
		return exactTraceInt(int64(v)), true
	case int8:
		return exactTraceInt(int64(v)), true
	case int16:
		return exactTraceInt(int64(v)), true
	case int32:
		return exactTraceInt(int64(v)), true
	case int64:
		return exactTraceInt(v), true
	case uint:
		return traceUintNumber(uint64(v))
	case uint8:
		return traceUintNumber(uint64(v))
	case uint16:
		return traceUintNumber(uint64(v))
	case uint32:
		return traceUintNumber(uint64(v))
	case uint64:
		return traceUintNumber(v)
	case float32:
		return traceFloatNumber(float64(v)), true
	case float64:
		return traceFloatNumber(v), true
	case string:
		raw := strings.TrimSpace(v)
		if number, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return exactTraceInt(number), true
		}
		number, err := strconv.ParseFloat(raw, 64)
		return traceFloatNumber(number), err == nil
	default:
		return traceNumber{}, false
	}
}

func exactTraceInt(value int64) traceNumber {
	return traceNumber{floatValue: float64(value), intValue: value, exactInt: true}
}

func traceFloatNumber(value float64) traceNumber {
	return traceNumber{floatValue: value}
}

func traceUintNumber(value uint64) (traceNumber, bool) {
	const maxInt64AsUint = uint64(1<<63 - 1)
	if value <= maxInt64AsUint {
		return exactTraceInt(int64(value)), true
	}
	return traceFloatNumber(float64(value)), true
}

func aggregateTraceSpanStatus(spans []map[string]interface{}) string {
	firstStatus := ""
	for _, span := range spans {
		status := firstItemString(span, "status")
		if status == "" {
			continue
		}
		if strings.EqualFold(status, "ERROR") {
			return "ERROR"
		}
		if firstStatus == "" {
			firstStatus = status
		}
	}
	return firstStatus
}

func normalizeTraceDetail(data map[string]interface{}) map[string]interface{} {
	trace := firstTraceMap(data, "trace", "trace_detail", "traceDetail")
	if trace == nil {
		trace = data
	}
	out := normalizeTraceObject(trace)
	if spans := firstMapSlice(trace, "spans", "span_items", "spanItems"); len(spans) > 0 {
		normalized := make([]map[string]interface{}, 0, len(spans))
		for _, span := range spans {
			normalized = append(normalized, normalizeTraceSpan(span))
		}
		out["spans"] = normalized
		if firstTraceString(out, "trace_id") == "" {
			if traceID := firstTraceString(normalized[0], "trace_id"); traceID != "" {
				out["trace_id"] = traceID
			}
		}
	}
	return out
}

func normalizeTraceObject(trace map[string]interface{}) map[string]interface{} {
	out := cloneMap(trace)
	normalizeObservabilityAttributes(out)
	copyFirstAlias(out, trace, "trace_id", "trace_id", "traceID", "traceId")
	copyFirstAlias(out, trace, "is_break", "is_break", "isBreak")
	return out
}

func normalizeTraceSpan(span map[string]interface{}) map[string]interface{} {
	out := cloneMap(span)
	normalizeObservabilityAttributes(out)
	copyFirstAlias(out, span, "trace_id", "trace_id", "traceID", "traceId")
	copyFirstAlias(out, span, "span_id", "span_id", "spanID", "spanId")
	copyFirstAlias(out, span, "parent_span_id", "parent_span_id", "parentSpanID", "parentSpanId")
	copyFirstAlias(out, span, "start_time_ns", "start_time_ns", "startTimeNs", "start_time_unix_nano", "startTimeUnixNano")
	copyFirstAlias(out, span, "end_time_ns", "end_time_ns", "endTimeNs", "end_time_unix_nano", "endTimeUnixNano")
	copyFirstAlias(out, span, "duration_ms", "duration_ms", "durationMs")
	copyFirstAlias(out, span, "is_break", "is_break", "isBreak")
	for _, key := range []string{"duration_ms", "user_id", "status", "module"} {
		if _, ok := out[key]; !ok {
			if value := appsAttributeValue(span["attributes"], key); value != nil {
				out[key] = value
			}
		}
	}
	return out
}

func traceListRows(items []map[string]interface{}) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		rows = append(rows, traceSummaryRow(item))
	}
	return rows
}

var traceSummarySchema = appsOutputSchema{
	Columns: []appsOutputColumn{
		{Key: "start_time_ns", Label: "start-time", Format: appsFormatNS("2006-01-02 15:04:05.000")},
		{Key: "root_span", Label: "root-span"},
		{Key: "user_id", Label: "user-id"},
		{Key: "duration_ms", Label: "duration", Format: appsFormatDurationMS},
		{Key: "trace_id", Label: "trace-id"},
	},
	Strict: true,
}

func traceDetailSummary(trace map[string]interface{}) map[string]interface{} {
	if spans := traceMapSlice(trace["spans"]); len(spans) > 0 {
		summaries := aggregateTraceSpanSummaries(spans)
		if len(summaries) > 0 {
			summary := summaries[0]
			for _, key := range []string{"trace_id", "is_break"} {
				if value, ok := trace[key]; ok {
					summary[key] = value
				}
			}
			return summary
		}
	}
	return traceSummaryRow(trace)
}

func traceSummaryRow(item map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"trace_id":      item["trace_id"],
		"start_time_ns": item["start_time_ns"],
		"root_span":     firstItemString(item, "root_span", "name", "span_name"),
		"user_id":       item["user_id"],
		"duration_ms":   item["duration_ms"],
		"status":        item["status"],
		"span_count":    item["span_count"],
	}
}

func firstTraceMap(data map[string]interface{}, keys ...string) map[string]interface{} {
	for _, key := range keys {
		if value, ok := data[key].(map[string]interface{}); ok {
			return value
		}
	}
	return nil
}

func firstTraceString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := firstTraceValue(data, key); ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func firstTraceValue(data map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value, true
		}
	}
	return nil, false
}
