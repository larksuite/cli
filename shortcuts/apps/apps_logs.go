// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	defaultAppsLogEnv       = "online"
	logSearchEndpoint       = "search_logs"
	resolveStackEndpoint    = "resolve_stack_trace"
	sourceStackStatusOK     = "resolved"
	sourceStackStatusError  = "unresolved"
	sourceStackMaxScanDepth = 8
	sourceStackMaxFrames    = 2000
	defaultSourceMapPrefix  = "client/assets/"
)

var (
	jsStackFrameParenRe = regexp.MustCompile(`^\s*(?:at\s+(.+?)\s+)?\((.+):(\d+):(\d+)\)\s*$`)
	jsStackFrameBareRe  = regexp.MustCompile(`^\s*(?:at\s+)?(.+):(\d+):(\d+)\s*$`)
)

// AppsLogList searches online app logs with observability filters.
var AppsLogList = common.Shortcut{
	Service:     appsService,
	Command:     "+log-list",
	Description: "Search online app logs with observability filters",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +log-list --app-id <app_id> --level error --keyword timeout --since 1h",
		"Tip: use --page-token from the response to fetch the next page.",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID whose online logs should be searched", Required: true},
		{Name: appsEnvironmentFlag, Default: defaultAppsLogEnv, Desc: "observability environment; only online is supported"},
		{Name: "since", Desc: "start time, relative duration (30s, 5m, 0.5h, 2h, 3d, 1w), local date/time, or RFC3339"},
		{Name: "until", Desc: "end time, relative duration (30s, 5m, 0.5h, 2h, 3d, 1w), local date/time, or RFC3339"},
		{Name: "level", Type: "string_array", Desc: "log level filter; repeatable, one of DEBUG, INFO, WARN, ERROR (case-insensitive)"},
		{Name: "trace-id", Type: "string_array", Desc: "trace ID filter; repeatable"},
		{Name: "keyword", Desc: "keyword filter applied by the log search backend"},
		{Name: "module", Desc: "module name filter"},
		{Name: "user-id", Desc: "end user ID filter"},
		{Name: "page", Desc: "frontend page or route filter"},
		{Name: "api", Desc: "API path/name filter"},
		{Name: "min-duration", Type: "int", Desc: "minimum duration in milliseconds; must be non-negative"},
		{Name: "max-duration", Type: "int", Desc: "maximum duration in milliseconds; must be non-negative and >= --min-duration"},
		{Name: "page-size", Type: "int", Default: fmt.Sprintf("%d", defaultAppsPageSize), Desc: "page size, 1..100"},
		{Name: "page-token", Desc: "pagination cursor from a previous log search response"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		_, err := buildLogSearchBody(rctx)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildLogSearchBody(rctx)
		return common.NewDryRunAPI().
			POST(logSearchPath(rctx.Str("app-id"))).
			Desc("Search online app logs").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, _ := requireAppID(rctx.Str("app-id"))
		body, err := buildLogSearchBody(rctx)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", logSearchPath(appID), nil, body)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := normalizeLogSearchResponse(data)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			appsPrintSchemaTable(w, appsProjectRows(logListRows(out.Items), logSummarySchema), logSummarySchema)
		})
		return nil
	},
}

// AppsLogGet fetches one log by log ID through the search_logs endpoint.
var AppsLogGet = common.Shortcut{
	Service:     appsService,
	Command:     "+log-get",
	Description: "Get one online app log by log ID",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +log-get --app-id <app_id> --log-id <log_id>",
		"Tip: +log-get searches online logs with limit=1; use +log-list first if the log ID is unknown.",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID whose online logs should be searched", Required: true},
		{Name: "log-id", Desc: "log ID to fetch", Required: true},
		{Name: appsEnvironmentFlag, Default: defaultAppsLogEnv, Desc: "observability environment; only online is supported"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("log-id")) == "" {
			return appsValidationParamError("--log-id", "--log-id is required")
		}
		return validateObservabilityEnv(rctx.Str(appsEnvironmentFlag))
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST(logSearchPath(rctx.Str("app-id"))).
			Desc("Search online app logs by log ID").
			Body(buildLogGetSearchBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, _ := requireAppID(rctx.Str("app-id"))
		data, err := callLogGetSearch(rctx, appID, buildLogGetSearchBody(rctx))
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := normalizeLogSearchResponse(data)
		if len(out.Items) == 0 {
			return appsFailedPreconditionParamError("--log-id", "log not found").
				WithHint("verify --log-id and --environment online")
		}
		log := out.Items[0]
		enrichLogSourceStack(rctx, appID, log)
		rctx.OutFormat(log, nil, func(w io.Writer) {
			appsPrintSchemaTable(w, appsProjectRows([]map[string]interface{}{logSummaryRow(log)}, logSummarySchema), logSummarySchema)
		})
		return nil
	},
}

func callLogGetSearch(rctx *common.RuntimeContext, appID string, body map[string]interface{}) (map[string]interface{}, error) {
	resp, err := rctx.DoAPI(&larkcore.ApiReq{
		HttpMethod: "POST",
		ApiPath:    logSearchPath(appID),
		Body:       body,
	})
	if err != nil {
		return nil, err
	}
	data, err := rctx.ClassifyAPIResponse(resp)
	if err == nil && data != nil {
		return data, nil
	}
	if flex, ok := flexibleLogSearchData(resp.RawBody); ok && (err == nil || isNonObjectInvalidResponse(err)) {
		return flex, nil
	}
	return data, err
}

type logSearchOutput struct {
	Items     []map[string]interface{} `json:"items"`
	PageToken string                   `json:"page_token,omitempty"`
	HasMore   bool                     `json:"has_more"`
}

func logSearchPath(appID string) string {
	return appScopedPath(appID, logSearchEndpoint)
}

func resolveStackPath(appID string) string {
	return appScopedPath(appID, resolveStackEndpoint)
}

func buildLogSearchBody(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	env := strings.TrimSpace(rctx.Str(appsEnvironmentFlag))
	if env == "" {
		env = defaultAppsLogEnv
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
	if err := addLogSearchTimeRange(body, rctx); err != nil {
		return nil, err
	}
	filter, err := buildLogSearchFilter(rctx)
	if err != nil {
		return nil, err
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	return body, nil
}

func buildLogGetSearchBody(rctx *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"app_env": appsObservabilityBackendEnv,
		"limit":   1,
		"filter": map[string]interface{}{
			"log_ids": []string{strings.TrimSpace(rctx.Str("log-id"))},
		},
	}
}

func addLogSearchTimeRange(body map[string]interface{}, rctx *common.RuntimeContext) error {
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

func buildLogSearchFilter(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	filter := make(map[string]interface{})
	levels, err := normalizeLogLevels(rctx.StrArray("level"))
	if err != nil {
		return nil, err
	}
	if len(levels) > 0 {
		filter["levels"] = levels
	}
	if traceIDs := cleanRepeatedStrings(rctx.StrArray("trace-id")); len(traceIDs) > 0 {
		filter["trace_ids"] = traceIDs
	}
	addTrimmedLogFilterString(filter, "keyword", rctx.Str("keyword"))
	addTrimmedLogFilterStrings(filter, "modules", rctx.Str("module"))
	addTrimmedLogFilterStrings(filter, "user_ids", rctx.Str("user-id"))
	addTrimmedLogFilterStrings(filter, "pages", rctx.Str("page"))
	addTrimmedLogFilterStrings(filter, "apis", rctx.Str("api"))
	if err := addDurationFilters(filter, rctx); err != nil {
		return nil, err
	}
	return filter, nil
}

func addTrimmedLogFilterStrings(filter map[string]interface{}, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		filter[key] = []string{value}
	}
}

func addTrimmedLogFilterString(filter map[string]interface{}, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		filter[key] = value
	}
}

func addDurationFilters(filter map[string]interface{}, rctx *common.RuntimeContext) error {
	hasMin := rctx.Changed("min-duration")
	hasMax := rctx.Changed("max-duration")
	minDuration := rctx.Int("min-duration")
	maxDuration := rctx.Int("max-duration")
	if hasMin {
		if minDuration < 0 {
			return appsValidationParamError("--min-duration", "--min-duration must be non-negative")
		}
		filter["min_duration_ms"] = minDuration
	}
	if hasMax {
		if maxDuration < 0 {
			return appsValidationParamError("--max-duration", "--max-duration must be non-negative")
		}
		filter["max_duration_ms"] = maxDuration
	}
	if hasMin && hasMax && minDuration > maxDuration {
		return appsValidationParamError("--max-duration", "--max-duration must be greater than or equal to --min-duration")
	}
	return nil
}

func normalizeLogLevels(values []string) ([]string, error) {
	values = cleanRepeatedStrings(values)
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		level := strings.ToUpper(strings.TrimSpace(value))
		switch level {
		case "DEBUG", "INFO", "WARN", "ERROR":
			out = append(out, level)
		default:
			return nil, appsValidationParamError("--level", "--level must be one of DEBUG, INFO, WARN, ERROR")
		}
	}
	return out, nil
}

func normalizeLogSearchResponse(data map[string]interface{}) logSearchOutput {
	items := firstMapSlice(data, "items", "log_items", "logItems")
	normalized := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		normalized = append(normalized, normalizeLogItem(item))
	}
	return logSearchOutput{
		Items:     normalized,
		PageToken: firstLogString(data, "page_token", "next_page_token", "pageToken", "nextPageToken"),
		HasMore:   firstLogBool(data, "has_more", "hasMore"),
	}
}

func normalizeLogItem(item map[string]interface{}) map[string]interface{} {
	out := cloneMap(item)
	normalizeObservabilityAttributes(out)
	copyFirstAlias(out, item, "log_id", "log_id", "id", "logID", "logId")
	copyFirstAlias(out, item, "trace_id", "trace_id", "traceID", "traceId")
	copyFirstAlias(out, item, "timestamp_ns", "timestamp_ns", "timestampNs")
	copyFirstAlias(out, item, "severity_text", "severity_text", "severityText")
	if level := firstItemString(out, "level", "severity_text", "severityText"); level != "" {
		out["level"] = level
	}
	return out
}

func firstMapSlice(data map[string]interface{}, keys ...string) []map[string]interface{} {
	for _, key := range keys {
		raw, ok := data[key]
		if !ok {
			continue
		}
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
		}
	}
	return nil
}

func flexibleLogSearchData(raw []byte) (map[string]interface{}, bool) {
	var result interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, false
	}
	switch value := result.(type) {
	case []interface{}:
		return map[string]interface{}{"items": value}, true
	case map[string]interface{}:
		data, ok := value["data"]
		if !ok {
			return nil, false
		}
		items, ok := data.([]interface{})
		if !ok {
			return nil, false
		}
		out := map[string]interface{}{"items": items}
		for _, key := range []string{"page_token", "next_page_token", "pageToken", "nextPageToken", "has_more", "hasMore"} {
			if v, present := value[key]; present {
				out[key] = v
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func isNonObjectInvalidResponse(err error) bool {
	p, ok := errs.ProblemOf(err)
	return ok && p.Category == errs.CategoryInternal && p.Subtype == errs.SubtypeInvalidResponse
}

func firstLogString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if s, ok := data[key].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func firstLogBool(data map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if b, ok := data[key].(bool); ok {
			return b
		}
	}
	return false
}

func copyFirstAlias(dst, src map[string]interface{}, canonical string, keys ...string) {
	for _, key := range keys {
		if value, ok := src[key]; ok {
			dst[canonical] = value
			return
		}
	}
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src)+4)
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func logListRows(items []map[string]interface{}) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		rows = append(rows, logSummaryRow(item))
	}
	return rows
}

var logSummarySchema = appsOutputSchema{
	Columns: []appsOutputColumn{
		{Key: "timestamp_ns", Label: "time", Format: appsFormatNS("2006-01-02 15:04:05.000")},
		{Key: "level"},
		{Key: "module"},
		{Key: "user_id"},
		{Key: "duration_ms", Format: appsFormatDurationMS},
		{Key: "trace_id"},
		{Key: "log_id"},
		{Key: "message"},
	},
	Strict: true,
}

func logSummaryRow(item map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"log_id":       item["log_id"],
		"level":        firstItemString(item, "level", "severity_text"),
		"trace_id":     item["trace_id"],
		"timestamp_ns": item["timestamp_ns"],
		"module":       firstLogDetailValue(item, "module"),
		"user_id":      firstLogDetailValue(item, "user_id"),
		"duration_ms":  firstLogDetailValue(item, "duration_ms"),
		"message":      firstItemString(item, "message", "body"),
	}
}

func firstLogDetailValue(item map[string]interface{}, key string) interface{} {
	if value, ok := item[key]; ok {
		return value
	}
	return appsAttributeValue(item["attributes"], key)
}

func firstItemString(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if s, ok := item[key].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func enrichLogSourceStack(rctx *common.RuntimeContext, appID string, log map[string]interface{}) {
	if !shouldResolveSourceStack(log) {
		return
	}
	body, ok := extractSourceStackResolveBody(log)
	if !ok {
		log["source_stack_status"] = sourceStackStatusError
		log["source_stack_reason"] = "source stack fields incomplete"
		return
	}
	data, err := rctx.CallAPITyped("POST", resolveStackPath(appID), nil, body)
	if err != nil {
		if _, typed := errs.ProblemOf(err); typed {
			markSourceStackResolveError(log, err)
		}
		return
	}
	stack := firstLogValue(data, "source_stack", "sourceStack", "frames")
	if stack == nil {
		stack = data
	}
	log["source_stack_status"] = sourceStackStatusOK
	log["source_stack"] = stack
}

func markSourceStackResolveError(log map[string]interface{}, err error) {
	log["source_stack_status"] = sourceStackStatusError
	log["source_stack_reason"] = "resolve_stack_trace failed"
	if problem, ok := errs.ProblemOf(err); ok {
		if problem.Code != 0 {
			log["source_stack_error_code"] = problem.Code
			log["source_stack_reason"] = fmt.Sprintf("resolve_stack_trace failed: code %d", problem.Code)
		}
		if problem.LogID != "" {
			log["source_stack_log_id"] = problem.LogID
		}
	}
}

func shouldResolveSourceStack(log map[string]interface{}) bool {
	level := strings.ToUpper(firstItemString(log, "level", "severity_text", "severityText"))
	if level != "ERROR" {
		return false
	}
	if _, ok := extractSourceStackResolveBody(log); ok {
		return true
	}
	return hasFrontendSourceMapSignal(log)
}

func hasFrontendSourceMapSignal(value interface{}) bool {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, nested := range v {
			if isSourceMapSignal(key) || hasFrontendSourceMapSignal(nested) {
				return true
			}
		}
	case []interface{}:
		for _, nested := range v {
			if hasFrontendSourceMapSignal(nested) {
				return true
			}
		}
	case string:
		return isSourceMapSignal(v) || strings.Contains(strings.ToLower(v), ".js")
	}
	return false
}

func isSourceMapSignal(value string) bool {
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(value))
	return strings.Contains(normalized, "source_map") || strings.Contains(normalized, "sourcemap")
}

func extractSourceStackResolveBody(log map[string]interface{}) (map[string]interface{}, bool) {
	sources := collectSourceStackMaps(log)
	commitID := firstStringInMaps(sources, "commit_id", "commitID", "commitId", "release_commit_id", "releaseCommitID", "releaseCommitId")
	prefix := firstStringInMaps(sources, "source_map_file_prefix", "sourceMapFilePrefix", "source_map_prefix", "sourceMapPrefix")
	if prefix == "" && firstStringInMaps(sources, "release_commit_id", "releaseCommitID", "releaseCommitId") != "" {
		prefix = defaultSourceMapPrefix
	}
	frames := firstFramesInMaps(
		sources,
		"frames",
		"stack_frames",
		"stackFrames",
		"source_stack_frames",
		"sourceStackFrames",
		"stack",
		"stack_trace",
		"stackTrace",
		"error_stack",
		"errorStack",
		"exception_stack",
		"exceptionStack",
		"message",
		"body",
	)
	if commitID == "" || prefix == "" || len(frames) == 0 {
		return nil, false
	}
	body := map[string]interface{}{
		"commit_id":              commitID,
		"source_map_file_prefix": prefix,
		"frames":                 frames,
	}
	if tenantID := firstStringInMaps(sources, "tenant_id", "tenantID", "tenantId"); tenantID != "" {
		body["tenant_id"] = tenantID
	}
	return body, true
}

func collectSourceStackMaps(value interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, 8)
	collectSourceStackMapsInto(value, 0, &out)
	return out
}

func collectSourceStackMapsInto(value interface{}, depth int, out *[]map[string]interface{}) {
	if depth > sourceStackMaxScanDepth || value == nil {
		return
	}
	switch v := value.(type) {
	case map[string]interface{}:
		*out = append(*out, v)
		for _, nested := range v {
			collectSourceStackMapsInto(nested, depth+1, out)
		}
	case []interface{}:
		if attrs := observabilityKVList(v); len(attrs) > 0 {
			*out = append(*out, attrs)
			for _, nested := range attrs {
				collectSourceStackMapsInto(nested, depth+1, out)
			}
		}
		for _, nested := range v {
			collectSourceStackMapsInto(nested, depth+1, out)
		}
	case []map[string]interface{}:
		for _, nested := range v {
			collectSourceStackMapsInto(nested, depth+1, out)
		}
	case string:
		if parsed := parseJSONObjectString(v); parsed != nil {
			collectSourceStackMapsInto(parsed, depth+1, out)
		}
	}
}

func firstStringInMaps(sources []map[string]interface{}, keys ...string) string {
	for _, source := range sources {
		if s := firstLogString(source, keys...); s != "" {
			return s
		}
	}
	return ""
}

func firstFramesInMaps(sources []map[string]interface{}, keys ...string) []interface{} {
	for _, key := range keys {
		for _, source := range sources {
			frames := normalizeFrames(source[key])
			if len(frames) > 0 {
				return frames
			}
		}
	}
	return nil
}

func normalizeFrames(raw interface{}) []interface{} {
	switch frames := raw.(type) {
	case []interface{}:
		out := make([]interface{}, 0, len(frames))
		for _, frame := range frames {
			if normalized, ok := normalizeFrame(frame); ok {
				out = append(out, normalized)
				if len(out) >= sourceStackMaxFrames {
					return out
				}
			}
		}
		return out
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(frames))
		for _, frame := range frames {
			if normalized, ok := normalizeFrame(frame); ok {
				out = append(out, normalized)
				if len(out) >= sourceStackMaxFrames {
					return out
				}
			}
		}
		return out
	case string:
		return parseFrameString(frames)
	default:
		return nil
	}
}

func normalizeFrame(frame interface{}) (map[string]interface{}, bool) {
	switch f := frame.(type) {
	case map[string]interface{}:
		return normalizeFrameMap(f)
	case map[string]string:
		m := make(map[string]interface{}, len(f))
		for key, value := range f {
			m[key] = value
		}
		return normalizeFrameMap(m)
	case string:
		parsed := parseJSStackFrameLine(f)
		if _, ok := parsed["file_name"]; !ok {
			return nil, false
		}
		return parsed, true
	default:
		return nil, false
	}
}

func normalizeFrameMap(frame map[string]interface{}) (map[string]interface{}, bool) {
	fileName := normalizeSourceFrameFileName(firstLogString(frame, "file_name", "fileName", "filename", "file", "url"))
	line, lineOK := firstFrameInt(frame, "line", "line_number", "lineNumber")
	column, columnOK := firstFrameInt(frame, "column", "col", "column_number", "columnNumber")
	if fileName == "" || !lineOK || !columnOK {
		return nil, false
	}
	out := map[string]interface{}{
		"file_name": fileName,
		"line":      line,
		"column":    column,
	}
	if fn := firstLogString(frame, "function", "function_name", "functionName", "method", "methodName"); fn != "" {
		out["function"] = fn
	}
	return out, true
}

func normalizeSourceFrameFileName(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return ""
	}
	parts := strings.FieldsFunc(fileName, func(r rune) bool {
		return r == '/' || r == '?' || r == '#'
	})
	for i := len(parts) - 1; i >= 0; i-- {
		if part := strings.TrimSpace(parts[i]); part != "" {
			return part
		}
	}
	return fileName
}

func firstFrameInt(frame map[string]interface{}, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := frame[key]; ok {
			if n, valid := frameInt(value); valid {
				return n, true
			}
		}
	}
	return 0, false
}

func frameInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return positiveFrameInt(v)
	case int64:
		if v > int64(^uint(0)>>1) {
			return 0, false
		}
		return positiveFrameInt(int(v))
	case float64:
		if v != float64(int(v)) {
			return 0, false
		}
		return positiveFrameInt(int(v))
	case json.Number:
		n, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, false
		}
		return positiveFrameInt(n)
	case string:
		return parsePositiveInt(v)
	default:
		return 0, false
	}
}

func positiveFrameInt(n int) (int, bool) {
	if n < 1 {
		return 0, false
	}
	return n, true
}

func parseFrameString(raw string) []interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var decoded []interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return normalizeFrames(decoded)
	}
	lines := strings.Split(raw, "\n")
	out := make([]interface{}, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if frame, ok := normalizeFrame(parseJSStackFrameLine(line)); ok {
			out = append(out, frame)
			if len(out) >= sourceStackMaxFrames {
				return out
			}
		}
	}
	return out
}

func parseJSStackFrameLine(line string) map[string]interface{} {
	if frame := parseJSStackFrameMatch(line, jsStackFrameParenRe.FindStringSubmatch(line)); frame != nil {
		return frame
	}
	if frame := parseJSStackFrameMatch(line, jsStackFrameBareRe.FindStringSubmatch(line)); frame != nil {
		return frame
	}
	return map[string]interface{}{"raw": line}
}

func parseJSStackFrameMatch(raw string, match []string) map[string]interface{} {
	if match == nil {
		return nil
	}
	switch len(match) {
	case 4:
		line, lineOK := parsePositiveInt(match[2])
		column, columnOK := parsePositiveInt(match[3])
		if lineOK && columnOK {
			return map[string]interface{}{"file_name": normalizeSourceFrameFileName(match[1]), "line": line, "column": column}
		}
	case 5:
		line, lineOK := parsePositiveInt(match[3])
		column, columnOK := parsePositiveInt(match[4])
		if lineOK && columnOK {
			out := map[string]interface{}{
				"file_name": normalizeSourceFrameFileName(match[2]),
				"line":      line,
				"column":    column,
			}
			if fn := strings.TrimSpace(match[1]); fn != "" {
				out["function"] = fn
			}
			return out
		}
	}
	return map[string]interface{}{"raw": raw}
}

func parseJSONObjectString(raw string) map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	return parsed
}

func parsePositiveInt(raw string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func firstLogValue(data map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value
		}
	}
	return nil
}
