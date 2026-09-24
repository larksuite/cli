// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// validateProgressDataConfig checks the raw create object without changing it.
// Defaults and field-type-dependent filter validation belong to the server;
// update cannot use this validator because it has no current mode or sources.
func validateProgressDataConfig(cfg map[string]interface{}) []string {
	problems := progressObjectFields(cfg, "data_config", "progress")
	progress, ok := cfg["progress"].(map[string]interface{})
	if !ok {
		return append(problems, "progress 必须是包含 type、current、target 的对象")
	}
	mode, _ := progress["type"].(string)
	switch mode {
	case "value":
		problems = append(problems, progressObjectFields(progress, "progress", "type", "current", "target")...)
		problems = append(problems, validateProgressValueSource(progress["current"], "progress.current")...)
		problems = append(problems, validateProgressValueSource(progress["target"], "progress.target")...)
	case "date":
		problems = append(problems, progressObjectFields(progress, "progress", "type", "current", "target", "dateFormat")...)
		problems = append(problems, validateProgressDateSource(progress["current"], "progress.current", false)...)
		problems = append(problems, validateProgressDateSource(progress["target"], "progress.target", true)...)
		if raw, exists := progress["dateFormat"]; exists {
			format, ok := raw.(string)
			if !ok || !slices.Contains([]string{"YYYY-MM-DD", "YYYY/MM/DD", "MM-DD", "MM/DD/YYYY", "DD/MM/YYYY"}, format) {
				problems = append(problems, "progress.dateFormat 仅支持 YYYY-MM-DD|YYYY/MM/DD|MM-DD|MM/DD/YYYY|DD/MM/YYYY")
			}
		}
	default:
		problems = append(problems, "progress.type 必填且仅支持 value|date")
	}
	return problems
}

func validateProgressValueSource(raw interface{}, path string) []string {
	source, ok := raw.(map[string]interface{})
	if !ok {
		return []string{path + " 必须是完整的 custom 或 field 来源对象"}
	}
	typeName, _ := source["type"].(string)
	switch typeName {
	case "custom":
		problems := progressObjectFields(source, path, "type", "value")
		if value, ok := source["value"].(float64); !ok || math.IsNaN(value) || math.IsInf(value, 0) {
			problems = append(problems, path+".value 必须是有限数字，允许 0、负数和小数")
		}
		return problems
	case "field":
		problems := progressObjectFields(source, path, "type", "table_name", "count_all", "series", "filter")
		if name, _ := source["table_name"].(string); strings.TrimSpace(name) == "" {
			problems = append(problems, path+".table_name 必须是非空表名")
		}
		count, hasCount := source["count_all"]
		series, hasSeries := source["series"]
		if hasCount == hasSeries {
			problems = append(problems, path+" 必须且只能提供 count_all:true 或 series")
		}
		if hasCount && count != true {
			problems = append(problems, path+".count_all 只能为 true")
		}
		if hasSeries {
			items, ok := series.([]interface{})
			if !ok || len(items) != 1 {
				problems = append(problems, path+".series 必须恰好包含 1 个指标")
			} else if item, ok := items[0].(map[string]interface{}); !ok {
				problems = append(problems, path+".series[0] 必须是对象")
			} else {
				problems = append(problems, progressObjectFields(item, path+".series[0]", "field_name", "rollup")...)
				if name, _ := item["field_name"].(string); strings.TrimSpace(name) == "" {
					problems = append(problems, path+".series[0].field_name 必须是非空字段名")
				}
				rollup, _ := item["rollup"].(string)
				if !slices.Contains([]string{"SUM", "MAX", "MIN", "AVERAGE"}, rollup) {
					problems = append(problems, path+".series[0].rollup 仅支持 SUM|MAX|MIN|AVERAGE；记录数使用 count_all:true")
				}
			}
		}
		return append(problems, validateProgressFilter(source, path)...)
	default:
		return []string{path + ".type 必填且仅支持 custom|field"}
	}
}

func validateProgressDateSource(raw interface{}, path string, target bool) []string {
	source, ok := raw.(map[string]interface{})
	if !ok {
		return []string{path + " 必须是完整的 custom 或 relative 来源对象"}
	}
	typeName, _ := source["type"].(string)
	switch typeName {
	case "relative":
		problems := progressObjectFields(source, path, "type", "value")
		allowed := []string{"today", "yesterday"}
		if target {
			allowed = []string{"month", "quarter", "year"}
		}
		value, _ := source["value"].(string)
		if !slices.Contains(allowed, value) {
			problems = append(problems, path+".value 仅支持 "+strings.Join(allowed, "|"))
		}
		return problems
	case "custom":
		if !target {
			problems := progressObjectFields(source, path, "type", "value")
			if !isProgressTimestamp(source["value"]) {
				problems = append(problems, path+".value 必须是安全整数毫秒时间戳")
			}
			return problems
		}
		problems := progressObjectFields(source, path, "type", "range")
		values, ok := source["range"].([]interface{})
		if !ok || len(values) != 2 || !isProgressTimestamp(values[0]) || !isProgressTimestamp(values[1]) {
			return append(problems, path+".range 必须恰好包含 2 个安全整数毫秒时间戳")
		}
		if values[0].(float64) >= values[1].(float64) {
			problems = append(problems, path+".range 的起点必须小于终点")
		}
		return problems
	default:
		return []string{path + ".type 必填且仅支持 custom|relative，日期模式不支持 field"}
	}
}

func isProgressTimestamp(raw interface{}) bool {
	value, ok := raw.(float64)
	return ok && !math.IsNaN(value) && math.Abs(value) <= 9007199254740991 && math.Trunc(value) == value
}

func progressObjectFields(object map[string]interface{}, path string, allowed ...string) []string {
	var problems []string
	for _, key := range unexpectedObjectFields(object, allowed...) {
		problems = append(problems, path+" 不支持字段 "+key)
	}
	return problems
}

func validateProgressFilter(source map[string]interface{}, path string) []string {
	var problems []string
	for _, problem := range validateProtocolFilter(source, "filter") {
		problems = append(problems, path+"."+problem)
	}
	if filter, ok := source["filter"].(map[string]interface{}); ok {
		problems = append(problems, progressObjectFields(filter, path+".filter", "conjunction", "conditions")...)
		if conditions, ok := filter["conditions"].([]interface{}); ok {
			for i, raw := range conditions {
				if condition, ok := raw.(map[string]interface{}); ok {
					problems = append(problems, progressObjectFields(condition,
						fmt.Sprintf("%s.filter.conditions[%d]", path, i), "field_name", "operator", "value")...)
				}
			}
		}
	}
	return problems
}
