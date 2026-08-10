// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/base/recordexport"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	recordFilterJSONFlag = "filter-json"
	recordSortJSONFlag   = "sort-json"
	recordSortMaxCount   = 10
)

func recordFilterFlag() common.Flag {
	return common.Flag{
		Name:  recordFilterJSONFlag,
		Desc:  `filter JSON object or @file, same shape as view filter JSON; overrides --view-id view filters`,
		Input: []string{common.File},
	}
}

func recordSortFlag() common.Flag {
	return common.Flag{
		Name:  recordSortJSONFlag,
		Desc:  `sort JSON array or @file, e.g. [{"field":"Updated","desc":true}]; also accepts {"sort_config":[...]}; order is priority; max 10`,
		Input: []string{common.File},
	}
}

func validateRecordQueryOptions(runtime *common.RuntimeContext) error {
	if _, err := parseRecordFilterFlag(runtime); err != nil {
		return err
	}
	_, err := parseRecordSortFlag(runtime)
	return err
}

func parseRecordFilterFlag(runtime *common.RuntimeContext) (interface{}, error) {
	filterRaw := strings.TrimSpace(runtime.Str(recordFilterJSONFlag))
	if filterRaw == "" {
		return nil, nil
	}
	pc := newParseCtx(runtime)
	return parseJSONObject(pc, filterRaw, recordFilterJSONFlag)
}

func parseRecordSortFlag(runtime *common.RuntimeContext) ([]interface{}, error) {
	sortRaw := strings.TrimSpace(runtime.Str(recordSortJSONFlag))
	if sortRaw == "" {
		return nil, nil
	}
	pc := newParseCtx(runtime)
	value, err := parseJSONValue(pc, sortRaw, recordSortJSONFlag)
	if err != nil {
		return nil, err
	}
	return normalizeRecordSortValue(value, "--"+recordSortJSONFlag)
}

func normalizeRecordSortValue(value interface{}, label string) ([]interface{}, error) {
	var sortConfig []interface{}
	if parsed, ok := value.([]interface{}); ok {
		sortConfig = parsed
	} else if obj, ok := value.(map[string]interface{}); ok {
		rawSortConfig, ok := obj["sort_config"]
		if !ok {
			return nil, baseFlagErrorf("%s must be a JSON array or an object with sort_config array", label)
		}
		parsed, ok := rawSortConfig.([]interface{})
		if !ok {
			return nil, baseFlagErrorf("%s.sort_config must be a JSON array", label)
		}
		sortConfig = parsed
	} else {
		return nil, baseFlagErrorf("%s must be a JSON array or an object with sort_config array", label)
	}
	if len(sortConfig) > recordSortMaxCount {
		return nil, baseFlagErrorf("sort supports at most %d sort conditions; got %d", recordSortMaxCount, len(sortConfig))
	}
	return sortConfig, nil
}

func marshalRecordQueryFlag(flagName string, value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", baseFlagErrorf("--%s cannot encode JSON: %v", flagName, err)
	}
	return string(data), nil
}

func applyRecordQueryToParams(runtime *common.RuntimeContext, params map[string]interface{}) error {
	filter, err := parseRecordFilterFlag(runtime)
	if err != nil {
		return err
	}
	if filter != nil {
		filterJSON, err := marshalRecordQueryFlag(recordFilterJSONFlag, filter)
		if err != nil {
			return err
		}
		params["filter"] = filterJSON
	}
	sortConfig, err := parseRecordSortFlag(runtime)
	if err != nil {
		return err
	}
	if len(sortConfig) > 0 {
		sortJSON, err := marshalRecordQueryFlag(recordSortJSONFlag, sortConfig)
		if err != nil {
			return err
		}
		params["sort"] = sortJSON
	}
	return nil
}

func applyRecordQueryToURLValues(runtime *common.RuntimeContext, params url.Values) error {
	filter, err := parseRecordFilterFlag(runtime)
	if err != nil {
		return err
	}
	if filter != nil {
		filterJSON, err := marshalRecordQueryFlag(recordFilterJSONFlag, filter)
		if err != nil {
			return err
		}
		params["filter"] = []string{filterJSON}
	}
	sortConfig, err := parseRecordSortFlag(runtime)
	if err != nil {
		return err
	}
	if len(sortConfig) > 0 {
		sortJSON, err := marshalRecordQueryFlag(recordSortJSONFlag, sortConfig)
		if err != nil {
			return err
		}
		params["sort"] = []string{sortJSON}
	}
	return nil
}

func applyRecordQueryToBody(runtime *common.RuntimeContext, body map[string]interface{}) error {
	filter, err := parseRecordFilterFlag(runtime)
	if err != nil {
		return err
	}
	if filter != nil {
		body["filter"] = filter
	}
	sortConfig, err := parseRecordSortFlag(runtime)
	if err != nil {
		return err
	}
	if len(sortConfig) > 0 {
		body["sort"] = sortConfig
	}
	return nil
}

func recordSearchFlagBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if keyword := strings.TrimSpace(runtime.Str("keyword")); keyword != "" {
		body["keyword"] = keyword
	}
	searchFields := runtime.StrArray("search-field")
	if len(searchFields) > 0 {
		body["search_fields"] = searchFields
	}
	selectFields, err := recordSearchProjectionFields(runtime)
	if err != nil {
		return nil, err
	}
	if len(selectFields) > 0 {
		body["select_fields"] = selectFields
	}
	if viewID := runtime.Str("view-id"); viewID != "" {
		body["view_id"] = viewID
	}
	offset := runtime.Int("offset")
	if offset < 0 {
		offset = 0
	}
	body["offset"] = offset
	body["limit"] = runtime.Int("limit")
	return body, applyRecordQueryToBody(runtime, body)
}

func recordSearchJSONBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	pc := newParseCtx(runtime)
	body, err := parseJSONObject(pc, runtime.Str("json"), "json")
	if err != nil {
		return nil, err
	}
	if err := normalizeRecordSearchJSONBody(body); err != nil {
		return nil, err
	}
	if _, exists := body["limit"]; !exists && runtime.Str("format") == recordexport.FormatNDJSON {
		body["limit"] = maxNDJSONRecordReadLimit
	}
	return body, applyRecordQueryToBody(runtime, body)
}

func normalizeRecordSearchJSONBody(body map[string]interface{}) error {
	if rawSelectFields, ok := body["select_fields"]; ok {
		if rawSelectFields == nil {
			delete(body, "select_fields")
		} else {
			selectFields, err := normalizeRecordSearchSelectFields(rawSelectFields)
			if err != nil {
				return withValidationParam(err, "--json")
			}
			if len(selectFields) > 0 {
				body["select_fields"] = selectFields
			}
		}
	}
	if rawSort, ok := body["sort"]; ok {
		if sortConfig, err := normalizeRecordSortValue(rawSort, "--json.sort"); err == nil {
			body["sort"] = sortConfig
		} else {
			return err
		}
	}
	return nil
}

func validateRecordSearchFlags(runtime *common.RuntimeContext) error {
	if err := validateRecordReadFormat(runtime); err != nil {
		return err
	}
	if err := validateRecordExportFlags(runtime); err != nil {
		return err
	}
	jsonRaw := strings.TrimSpace(runtime.Str("json"))
	if jsonRaw != "" {
		if exclusiveParams := recordSearchJSONExclusiveFlagParams(runtime); len(exclusiveParams) > 0 {
			allParams := append([]string{"--json"}, exclusiveParams...)
			invalidParams := make([]errs.InvalidParam, 0, len(allParams))
			for _, param := range allParams {
				invalidParams = append(invalidParams, errs.InvalidParam{Name: param, Reason: "mutually exclusive"})
			}
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--json is mutually exclusive with %s",
				strings.Join(exclusiveParams, " and "),
			).
				WithParam("--json").
				WithParams(invalidParams...).
				WithHint("Put keyword, search, projection, view, and pagination fields inside --json, or omit --json.")
		}
		body, err := recordSearchJSONBody(runtime)
		if err != nil {
			return err
		}
		_, limit, err := recordSearchPagination(body)
		if err != nil {
			return withValidationParam(err, "--json")
		}
		maximum := maxInlineRecordReadLimit
		if runtime.Str("format") == "ndjson" {
			maximum = maxNDJSONRecordReadLimit
		}
		if limit > maximum {
			return withValidationParam(baseFlagErrorf("limit must be between 1 and %d; got %d", maximum, limit), "--json")
		}
		return nil
	}
	if strings.TrimSpace(runtime.Str("keyword")) == "" {
		return baseFlagErrorf("--keyword is required unless --json is used")
	}
	if len(runtime.StrArray("search-field")) == 0 {
		return baseFlagErrorf("--search-field is required unless --json is used")
	}
	if err := validateRecordReadLimit(runtime, 10); err != nil {
		return err
	}
	if _, err := recordSearchProjectionFields(runtime); err != nil {
		return err
	}
	return validateRecordQueryOptions(runtime)
}

func recordSearchPagination(body map[string]any) (int, int, error) {
	offset := 0
	if raw, exists := body["offset"]; exists {
		value, err := recordSearchInteger(raw, "offset")
		if err != nil {
			return 0, 0, err
		}
		if value < 0 {
			return 0, 0, baseFlagErrorf("offset must be greater than or equal to 0; got %d", value)
		}
		offset = value
	}
	limit := 10
	if raw, exists := body["limit"]; exists {
		value, err := recordSearchInteger(raw, "limit")
		if err != nil {
			return 0, 0, err
		}
		if value < 1 {
			return 0, 0, baseFlagErrorf("limit must be greater than or equal to 1; got %d", value)
		}
		limit = value
	}
	return offset, limit, nil
}

func recordSearchInteger(raw any, name string) (int, error) {
	switch value := raw.(type) {
	case int:
		return value, nil
	case int64:
		return int(value), nil
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, baseFlagErrorf("%s must be an integer; got %v", name, raw)
		}
		return int(parsed), nil
	case float64:
		if math.Trunc(value) != value {
			return 0, baseFlagErrorf("%s must be an integer; got %v", name, raw)
		}
		return int(value), nil
	default:
		return 0, baseFlagErrorf("%s must be an integer; got %T", name, raw)
	}
}

func recordSearchJSONExclusiveFlagParams(runtime *common.RuntimeContext) []string {
	names := []string{
		"keyword",
		"search-field",
		"field-id",
		"fields",
		"field-names",
		"view-id",
		"offset",
		"limit",
	}
	params := make([]string, 0, len(names))
	for _, name := range names {
		if runtime.Changed(name) {
			params = append(params, "--"+name)
		}
	}
	return params
}

func formatRecordQueryPriorityTip() string {
	return fmt.Sprintf("Query priority: --%s overrides --view-id's view filter JSON; --%s overrides --view-id's view sort config.", recordFilterJSONFlag, recordSortJSONFlag)
}
