// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"math"
	"strings"
)

// validateCountdownDataConfig checks complete create input at the JSON boundary.
// Preserve key presence and explicit null, and leave the input untouched:
// countdown uses camelCase and its defaults belong to the server. Updates skip
// this validator because their mode and table come from the existing block.
func validateCountdownDataConfig(cfg map[string]interface{}) []string {
	mode, _ := cfg["type"].(string)
	switch mode {
	case "fixed":
		problems := countdownObjectFields(cfg, "countdown.fixed", "type", "targetTime")
		target, ok := cfg["targetTime"].(float64)
		if !ok || math.IsNaN(target) || math.IsInf(target, 0) || target < 0 || target != math.Trunc(target) {
			problems = append(problems, "countdown.targetTime 必须是非负整数毫秒时间戳")
		}
		return problems
	case "field":
		problems := countdownObjectFields(cfg, "countdown.field", "type", "aggregateType", "additionalDatasources")
		if aggregate, _ := cfg["aggregateType"].(string); aggregate != "MIN" && aggregate != "MAX" {
			problems = append(problems, "countdown.aggregateType 必填且仅支持 MIN|MAX")
		}
		sources, ok := cfg["additionalDatasources"].([]interface{})
		if !ok || len(sources) != 1 {
			return append(problems, "countdown.additionalDatasources 必须且只能包含 1 个数据源")
		}
		source, ok := sources[0].(map[string]interface{})
		if !ok {
			return append(problems, "countdown.additionalDatasources[0] 必须是对象")
		}
		return append(problems, validateCountdownDatasource(source)...)
	default:
		return []string{"countdown.type 必填且仅支持 fixed|field"}
	}
}

func countdownObjectFields(object map[string]interface{}, path string, allowed ...string) []string {
	var problems []string
	for _, field := range unexpectedObjectFields(object, allowed...) {
		problems = append(problems, fmt.Sprintf("%s 不支持字段 %s", path, field))
	}
	return problems
}

func validateCountdownDatasource(source map[string]interface{}) []string {
	const path = "countdown.additionalDatasources[0]"
	problems := countdownObjectFields(source, path, "tableName", "countAll", "series", "groupBy", "filter")
	if table, _ := source["tableName"].(string); strings.TrimSpace(table) == "" {
		problems = append(problems, path+".tableName 必须是非空表名")
	}
	series, hasSeries := source["series"]
	countAll, hasCountAll := source["countAll"]
	if hasSeries == hasCountAll {
		problems = append(problems, path+" 的 series 与 countAll:true 必须二选一")
	}
	if hasCountAll && countAll != true {
		problems = append(problems, path+".countAll 只能为 true")
	}
	if hasSeries {
		items, ok := series.([]interface{})
		if !ok || len(items) < 1 || len(items) > 20 {
			problems = append(problems, path+".series 必须包含 1..20 个指标")
		} else {
			for i, raw := range items {
				itemPath := fmt.Sprintf("%s.series[%d]", path, i)
				item, ok := raw.(map[string]interface{})
				if !ok {
					problems = append(problems, itemPath+" 必须是对象")
					continue
				}
				problems = append(problems, countdownObjectFields(item, itemPath, "fieldName", "rollup")...)
				if field, _ := item["fieldName"].(string); strings.TrimSpace(field) == "" {
					problems = append(problems, itemPath+".fieldName 不能为空")
				}
				switch item["rollup"] {
				case "SUM", "MAX", "MIN", "AVERAGE":
				default:
					problems = append(problems, itemPath+".rollup 仅支持 SUM|MAX|MIN|AVERAGE")
				}
			}
		}
	}
	groups, ok := source["groupBy"].([]interface{})
	if !ok || len(groups) != 1 {
		problems = append(problems, path+".groupBy 必须且只能包含 1 个日期字段")
	} else {
		problems = append(problems, validateCountdownGroup(groups[0], path+".groupBy[0]")...)
	}
	if filter, exists := source["filter"]; exists {
		problems = append(problems, validateCountdownFilter(filter, path+".filter")...)
	}
	return problems
}

func validateCountdownGroup(raw interface{}, path string) []string {
	group, ok := raw.(map[string]interface{})
	if !ok {
		return []string{path + " 必须是对象"}
	}
	problems := countdownObjectFields(group, path, "fieldName", "mode", "sort")
	if field, _ := group["fieldName"].(string); strings.TrimSpace(field) == "" {
		problems = append(problems, path+".fieldName 不能为空；必须引用日期、创建时间或修改时间字段")
	}
	if mode, exists := group["mode"]; exists && mode != "integrated" && mode != "enumerated" {
		problems = append(problems, path+".mode 仅支持 integrated|enumerated")
	}
	if rawSort, exists := group["sort"]; exists {
		sortConfig, ok := rawSort.(map[string]interface{})
		if !ok {
			return append(problems, path+".sort 必须是对象")
		}
		problems = append(problems, countdownObjectFields(sortConfig, path+".sort", "type", "order")...)
		switch sortConfig["type"] {
		case "group", "value", "view":
		default:
			problems = append(problems, path+".sort.type 仅支持 group|value|view")
		}
		if order, exists := sortConfig["order"]; exists && order != "asc" && order != "desc" {
			problems = append(problems, path+".sort.order 仅支持 asc|desc")
		}
	}
	return problems
}

// Values and operators depend on the actual field type. Check the JSON shape
// here; the server resolves fields and validates their value semantics.
func validateCountdownFilter(raw interface{}, path string) []string {
	filter, ok := raw.(map[string]interface{})
	if !ok {
		return []string{path + " 必须是对象；filter:null 仅用于更新时清空筛选"}
	}
	problems := countdownObjectFields(filter, path, "conjunction", "conditions")
	if conjunction, _ := filter["conjunction"].(string); conjunction != "and" && conjunction != "or" {
		problems = append(problems, path+".conjunction 仅支持 and|or")
	}
	conditions, ok := filter["conditions"].([]interface{})
	if !ok || len(conditions) < 1 || len(conditions) > 50 {
		return append(problems, path+".conditions 必须包含 1..50 个条件")
	}
	for i, rawCondition := range conditions {
		conditionPath := fmt.Sprintf("%s.conditions[%d]", path, i)
		condition, ok := rawCondition.(map[string]interface{})
		if !ok {
			problems = append(problems, conditionPath+" 必须是对象")
			continue
		}
		problems = append(problems, countdownObjectFields(condition, conditionPath, "fieldName", "operator", "value")...)
		for _, key := range []string{"fieldName", "operator"} {
			if value, _ := condition[key].(string); strings.TrimSpace(value) == "" {
				problems = append(problems, conditionPath+"."+key+" 必须是非空字符串")
			}
		}
	}
	return problems
}
