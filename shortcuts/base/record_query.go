// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

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
	selectFields := recordListFields(runtime)
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
	body["limit"] = getPaginationLimit(runtime)
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
	return body, applyRecordQueryToBody(runtime, body)
}

func normalizeRecordSearchJSONBody(body map[string]interface{}) error {
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
	jsonRaw := strings.TrimSpace(runtime.Str("json"))
	if jsonRaw != "" {
		if recordSearchHasJSONExclusiveFlagInputs(runtime) {
			return baseFlagErrorf("--json is mutually exclusive with keyword/search/projection/pagination flags; put those fields inside --json, or omit --json")
		}
		_, err := recordSearchJSONBody(runtime)
		return err
	}
	if strings.TrimSpace(runtime.Str("keyword")) == "" {
		return baseFlagErrorf("--keyword is required unless --json is used")
	}
	if len(runtime.StrArray("search-field")) == 0 {
		return baseFlagErrorf("--search-field is required unless --json is used")
	}
	if err := validateLimitPageSizeAlias(runtime); err != nil {
		return err
	}
	if _, err := common.ValidatePageSizeTyped(runtime, "limit", 10, 1, 200); err != nil {
		return err
	}
	if runtime.Changed("page-size") {
		if _, err := common.ValidatePageSizeTyped(runtime, "page-size", 10, 1, 200); err != nil {
			return err
		}
	}
	return validateRecordQueryOptions(runtime)
}

func recordSearchHasJSONExclusiveFlagInputs(runtime *common.RuntimeContext) bool {
	return strings.TrimSpace(runtime.Str("keyword")) != "" ||
		len(runtime.StrArray("search-field")) > 0 ||
		len(recordListFields(runtime)) > 0 ||
		runtime.Str("view-id") != "" ||
		runtime.Changed("offset") ||
		runtime.Changed("limit") ||
		runtime.Changed("page-size")
}

func formatRecordQueryPriorityTip() string {
	return fmt.Sprintf("Query priority: --%s overrides --view-id's view filter JSON; --%s overrides --view-id's view sort config.", recordFilterJSONFlag, recordSortJSONFlag)
}
