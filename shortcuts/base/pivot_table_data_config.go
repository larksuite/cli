// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"strings"
)

var pivotGroupTypes = map[string]bool{
	"RAW": true, "DATE_YEAR": true, "DATE_YEAR_QUARTER": true,
	"DATE_YEAR_MONTH": true, "DATE_YEAR_MONTH_DAY": true,
	"DATE_YEAR_WEEK": true, "DATE_MONTH_DAY": true,
	"DATE_QUARTER": true, "DATE_MONTH": true, "DATE_WEEKDAY": true,
}

var pivotRollups = map[string]bool{
	"SUM": true, "COUNT": true, "COUNT_DISTINCT": true,
	"AVERAGE": true, "MAX": true, "MIN": true, "MEDIAN": true,
}

// normalizePivotTableDataConfig normalizes only the public pivot-table fields.
// Create materializes the three role arrays; update keeps top-level partial
// semantics and therefore never adds a field the caller did not send.
func normalizePivotTableDataConfig(cfg map[string]interface{}, create bool) map[string]interface{} {
	out := cloneMap(cfg)
	if create {
		for _, key := range []string{"rows", "columns", "values"} {
			if _, ok := out[key]; !ok {
				out[key] = []interface{}{}
			}
		}
	}
	for _, key := range []string{"rows", "columns"} {
		if items, ok := out[key].([]interface{}); ok {
			for _, raw := range items {
				if item, ok := raw.(map[string]interface{}); ok {
					if groupType, ok := item["group_type"].(string); ok {
						item["group_type"] = strings.ToUpper(strings.TrimSpace(groupType))
					}
				}
			}
		}
	}
	if items, ok := out["values"].([]interface{}); ok {
		for _, raw := range items {
			if item, ok := raw.(map[string]interface{}); ok {
				if rollup, ok := item["rollup"].(string); ok {
					item["rollup"] = strings.ToUpper(strings.TrimSpace(rollup))
				}
			}
		}
	}
	if sortConfigs, ok := out["sort"].([]interface{}); ok {
		for _, raw := range sortConfigs {
			if sortConfig, ok := raw.(map[string]interface{}); ok {
				if sortType, ok := sortConfig["sort_type"].(string); ok {
					sortConfig["sort_type"] = strings.ToUpper(strings.TrimSpace(sortType))
				}
				if order, ok := sortConfig["order"].(string); ok {
					sortConfig["order"] = strings.ToLower(strings.TrimSpace(order))
				}
			}
		}
	}
	return out
}

func isPivotTableDataConfig(cfg map[string]interface{}) bool {
	for _, key := range []string{"rows", "columns", "values", "view_name"} {
		if _, ok := cfg[key]; ok {
			return true
		}
	}
	if _, hasSort := cfg["sort"]; hasSort {
		return true
	}
	return false
}

func validatePivotTableDataConfig(cfg map[string]interface{}, create bool) []string {
	var problems []string
	if create {
		if tableName, _ := cfg["table_name"].(string); strings.TrimSpace(tableName) == "" {
			problems = append(problems, "缺少必填字段 table_name")
		}
	}
	if view, hasView := cfg["view_name"]; hasView && view != nil {
		if _, hasFilter := cfg["filter"]; hasFilter && cfg["filter"] != nil {
			problems = append(problems, "view_name 与 filter 互斥，不可同时存在")
		}
	}
	for _, key := range []string{"rows", "columns"} {
		problems = append(problems, validatePivotDimensions(cfg, key, create)...)
	}
	problems = append(problems, validatePivotValues(cfg, create)...)
	problems = append(problems, validatePivotSort(cfg, create)...)
	problems = append(problems, validateBlockFilter(cfg, "filter", false)...)
	return problems
}

func validatePivotDimensions(cfg map[string]interface{}, key string, create bool) []string {
	raw, exists := cfg[key]
	if !exists {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return []string{key + " 必须是数组"}
	}
	var problems []string
	for i, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("%s[%d] 必须是对象", key, i))
			continue
		}
		if fieldName, _ := item["field_name"].(string); strings.TrimSpace(fieldName) == "" {
			problems = append(problems, fmt.Sprintf("%s[%d].field_name 不能为空", key, i))
		}
		if create {
			if _, exists := item["field_key"]; exists {
				problems = append(problems, fmt.Sprintf("%s[%d].field_key 仅供更新已有配置使用，创建时请使用数组索引引用", key, i))
			}
		}
		if rawGroupType, exists := item["group_type"]; exists {
			groupType, _ := rawGroupType.(string)
			if !pivotGroupTypes[groupType] {
				problems = append(problems, fmt.Sprintf("%s[%d].group_type 不在允许枚举内: %s", key, i, groupType))
			}
		}
	}
	return problems
}

func validatePivotValues(cfg map[string]interface{}, create bool) []string {
	raw, exists := cfg["values"]
	if !exists {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return []string{"values 必须是数组"}
	}
	var problems []string
	for i, rawItem := range items {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("values[%d] 必须是对象", i))
			continue
		}
		if fieldName, _ := item["field_name"].(string); strings.TrimSpace(fieldName) == "" {
			problems = append(problems, fmt.Sprintf("values[%d].field_name 不能为空", i))
		}
		rollup, _ := item["rollup"].(string)
		if !pivotRollups[rollup] {
			problems = append(problems, fmt.Sprintf("values[%d].rollup 不在允许枚举内: %s", i, rollup))
		}
		if create {
			if _, exists := item["field_key"]; exists {
				problems = append(problems, fmt.Sprintf("values[%d].field_key 仅供更新已有配置使用，创建时请使用数组索引引用", i))
			}
		}
	}
	return problems
}

func validatePivotSort(cfg map[string]interface{}, create bool) []string {
	raw, exists := cfg["sort"]
	if !exists || raw == nil {
		return nil
	}
	sortConfigs, ok := raw.([]interface{})
	if !ok {
		return []string{"sort 必须是数组"}
	}
	var problems []string
	for i, rawSort := range sortConfigs {
		sortConfig, ok := rawSort.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("sort[%d] 必须是对象", i))
			continue
		}
		problems = append(problems, validatePivotSortItem(sortConfig, create, i)...)
	}
	return problems
}

func validatePivotSortItem(sortConfig map[string]interface{}, create bool, index int) []string {
	var problems []string
	sortType, _ := sortConfig["sort_type"].(string)
	if sortType != "FIELD" && sortType != "VALUE" {
		problems = append(problems, fmt.Sprintf("sort[%d].sort_type 仅支持 FIELD|VALUE", index))
	}
	order, _ := sortConfig["order"].(string)
	if order != "asc" && order != "desc" {
		problems = append(problems, fmt.Sprintf("sort[%d].order 仅支持 asc|desc", index))
	}
	problems = append(problems, validatePivotSortReference(sortConfig, "group", create, []string{"rows", "columns"})...)
	if sortType == "VALUE" {
		problems = append(problems, validatePivotSortReference(sortConfig, "value", create, []string{"values"})...)
	}
	return problems
}

func validatePivotSortReference(cfg map[string]interface{}, prefix string, create bool, allowedAreas []string) []string {
	keyName, refName := prefix+"_field_key", prefix+"_ref"
	key, _ := cfg[keyName].(string)
	ref, hasRef := cfg[refName]
	if create && strings.TrimSpace(key) != "" {
		return []string{keyName + " 仅供更新已有配置使用；创建时请传 " + refName}
	}
	if strings.TrimSpace(key) != "" && hasRef {
		return []string{keyName + " 与 " + refName + " 只能提供一个"}
	}
	if strings.TrimSpace(key) == "" && !hasRef {
		return []string{"sort 缺少 " + keyName + " 或 " + refName}
	}
	if !hasRef {
		return nil
	}
	refMap, ok := ref.(map[string]interface{})
	if !ok {
		return []string{refName + " 必须是对象"}
	}
	area, _ := refMap["area"].(string)
	validArea := false
	for _, allowed := range allowedAreas {
		validArea = validArea || area == allowed
	}
	index, validIndex := toIntStrict(refMap["index"])
	var problems []string
	if !validArea {
		problems = append(problems, refName+".area 不在允许范围内")
	}
	if !validIndex || index < 0 {
		problems = append(problems, refName+".index 必须是非负整数")
	}
	return problems
}
