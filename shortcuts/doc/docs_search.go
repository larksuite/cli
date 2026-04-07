// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var DocsSearch = common.Shortcut{
	Service:     "docs",
	Command:     "+search",
	Description: "Search Lark docs, Wiki, and spreadsheet files (Search v2: doc_wiki/search)",
	Risk:        "read",
	Scopes:      []string{"search:docs:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword"},
		{Name: "filter", Desc: "filter conditions (JSON object)"},
		{Name: "page-token", Desc: "page token"},
		{Name: "page-size", Default: "15", Desc: "page size (default 15, max 20)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		requestData, err := buildDocsSearchRequest(
			runtime.Str("query"),
			runtime.Str("filter"),
			runtime.Str("page-token"),
			runtime.Str("page-size"),
		)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		return common.NewDryRunAPI().
			POST("/open-apis/search/v2/doc_wiki/search").
			Body(requestData)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		requestData, err := buildDocsSearchRequest(
			runtime.Str("query"),
			runtime.Str("filter"),
			runtime.Str("page-token"),
			runtime.Str("page-size"),
		)
		if err != nil {
			return err
		}

		data, err := runtime.CallAPI("POST", "/open-apis/search/v2/doc_wiki/search", nil, requestData)
		if err != nil {
			return err
		}
		items, _ := data["res_units"].([]interface{})

		// Add ISO time fields
		normalizedItems := addIsoTimeFields(items)

		resultData := map[string]interface{}{
			"total":      data["total"],
			"has_more":   data["has_more"],
			"page_token": data["page_token"],
			"results":    normalizedItems,
		}

		runtime.OutFormat(resultData, &output.Meta{Count: len(normalizedItems)}, func(w io.Writer) {
			if len(normalizedItems) == 0 {
				fmt.Fprintln(w, "No matching results found.")
				return
			}

			// Table output
			htmlTagRe := regexp.MustCompile(`</?h>`)
			var rows []map[string]interface{}
			for _, item := range normalizedItems {
				u, _ := item.(map[string]interface{})
				if u == nil {
					continue
				}

				rawTitle := fmt.Sprintf("%v", u["title_highlighted"])
				title := htmlTagRe.ReplaceAllString(rawTitle, "")
				title = common.TruncateStr(title, 50)

				resultMeta, _ := u["result_meta"].(map[string]interface{})
				docTypes := ""
				if resultMeta != nil {
					docTypes = fmt.Sprintf("%v", resultMeta["doc_types"])
				}
				entityType := fmt.Sprintf("%v", u["entity_type"])
				typeStr := docTypes
				if typeStr == "" || typeStr == "<nil>" {
					typeStr = entityType
				}

				url := ""
				editTime := ""
				if resultMeta != nil {
					url = fmt.Sprintf("%v", resultMeta["url"])
					editTime = fmt.Sprintf("%v", resultMeta["update_time_iso"])
				}
				if len(url) > 80 {
					url = url[:80]
				}

				rows = append(rows, map[string]interface{}{
					"type":      typeStr,
					"title":     title,
					"edit_time": editTime,
					"url":       url,
				})
			}

			output.PrintTable(w, rows)
			moreHint := ""
			hasMore, _ := data["has_more"].(bool)
			if hasMore {
				moreHint = " (more available, use --format json to get page_token, then --page-token to paginate)"
			}
			fmt.Fprintf(w, "\n%d result(s)%s\n", len(rows), moreHint)
		})
		return nil
	},
}

func buildDocsSearchRequest(query, filterStr, pageToken, pageSizeStr string) (map[string]interface{}, error) {
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize <= 0 {
		pageSize = 15
	}
	if pageSize > 20 {
		pageSize = 20
	}

	requestData := map[string]interface{}{
		"query":     query,
		"page_size": pageSize,
	}
	if pageToken != "" {
		requestData["page_token"] = pageToken
	}

	if filterStr == "" {
		requestData["doc_filter"] = map[string]interface{}{}
		requestData["wiki_filter"] = map[string]interface{}{}
		return requestData, nil
	}

	var filter map[string]interface{}
	if err := json.Unmarshal([]byte(filterStr), &filter); err != nil {
		return nil, output.ErrValidation("--filter is not valid JSON")
	}
	if err := convertTimeRangeInFilter(filter, "open_time"); err != nil {
		return nil, err
	}
	if err := convertTimeRangeInFilter(filter, "create_time"); err != nil {
		return nil, err
	}

	hasFolderTokens := hasNonEmptyFilterArray(filter, "folder_tokens")
	hasSpaceIDs := hasNonEmptyFilterArray(filter, "space_ids")

	if hasFolderTokens && hasSpaceIDs {
		return nil, output.ErrValidation("--filter cannot contain both folder_tokens and space_ids; doc and wiki scoped search cannot be combined")
	}

	docFilter := cloneFilterMap(filter)
	delete(docFilter, "space_ids")

	wikiFilter := cloneFilterMap(filter)
	delete(wikiFilter, "folder_tokens")

	switch {
	case hasFolderTokens:
		requestData["doc_filter"] = docFilter
	case hasSpaceIDs:
		requestData["wiki_filter"] = wikiFilter
	default:
		requestData["doc_filter"] = docFilter
		requestData["wiki_filter"] = wikiFilter
	}
	return requestData, nil
}

func cloneFilterMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func hasNonEmptyFilterArray(filter map[string]interface{}, key string) bool {
	val, ok := filter[key]
	if !ok || val == nil {
		return false
	}
	items, ok := val.([]interface{})
	return ok && len(items) > 0
}

// convertTimeRangeInFilter converts ISO 8601 time range to Unix seconds.
func convertTimeRangeInFilter(filter map[string]interface{}, key string) error {
	val, ok := filter[key]
	if !ok {
		return nil
	}
	rangeMap, ok := val.(map[string]interface{})
	if !ok {
		return nil
	}

	result := make(map[string]interface{})
	if start, ok := rangeMap["start"].(string); ok && start != "" {
		startTime, err := toUnixSeconds(start)
		if err != nil {
			return output.ErrValidation("invalid %s.start %q: %s", key, start, err)
		}
		result["start"] = startTime
	}
	if end, ok := rangeMap["end"].(string); ok && end != "" {
		endTime, err := toUnixSeconds(end)
		if err != nil {
			return output.ErrValidation("invalid %s.end %q: %s", key, end, err)
		}
		result["end"] = endTime
	}
	filter[key] = result
	return nil
}

func toUnixSeconds(input string) (int64, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, input, time.Local); err == nil {
			return t.Unix(), nil
		}
	}
	// Try as number
	if n, err := strconv.ParseInt(input, 10, 64); err == nil {
		return n, nil
	}
	return 0, fmt.Errorf("expected RFC3339, YYYY-MM-DD[ HH:MM:SS], or unix seconds")
}

func unixTimestampToISO8601(v interface{}) string {
	if v == nil {
		return ""
	}

	var num float64
	switch val := v.(type) {
	case float64:
		num = val
	case json.Number:
		parsed, err := val.Float64()
		if err != nil {
			return ""
		}
		num = parsed
	case string:
		parsed, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return ""
		}
		num = parsed
	default:
		return ""
	}

	if math.IsInf(num, 0) || math.IsNaN(num) {
		return ""
	}

	// Heuristic: >= 1e12 treat as ms, else seconds
	ms := int64(num)
	if num >= 1e12 {
		ms = ms / 1000
	}
	t := time.Unix(ms, 0)
	return t.Format(time.RFC3339)
}

// addIsoTimeFields recursively adds *_time_iso fields.
func addIsoTimeFields(value interface{}) []interface{} {
	if arr, ok := value.([]interface{}); ok {
		result := make([]interface{}, len(arr))
		for i, item := range arr {
			result[i] = addIsoTimeFieldsOne(item)
		}
		return result
	}
	return nil
}

func addIsoTimeFieldsOne(value interface{}) interface{} {
	switch v := value.(type) {
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = addIsoTimeFieldsOne(item)
		}
		return result
	case map[string]interface{}:
		out := make(map[string]interface{})
		for key, item := range v {
			if strings.HasSuffix(key, "_time_iso") {
				out[key] = item
				continue
			}
			out[key] = addIsoTimeFieldsOne(item)
			if strings.HasSuffix(key, "_time") {
				iso := unixTimestampToISO8601(item)
				if iso != "" {
					out[key+"_iso"] = iso
				}
			}
		}
		return out
	default:
		return value
	}
}
