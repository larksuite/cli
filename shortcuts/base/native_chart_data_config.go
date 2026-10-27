// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"math"
)

// chartDataWithoutNativeStyle 剥离原生样式和标题元数据供数据校验，发送配置保持完整。
func chartDataWithoutNativeStyle(cfg map[string]interface{}) map[string]interface{} {
	data := make(map[string]interface{}, len(cfg))
	for key, value := range cfg {
		if key != "viewModel" && key != "overrideViewModel" && key != "name_style" {
			data[key] = value
		}
	}
	return data
}

// 原生字段保持 Web 命名；组件适用范围、依赖动作和完整叶子 schema 由服务端校验。
func validateNativeChartDataConfig(cfg map[string]interface{}) []string {
	var problems []string
	for _, root := range []string{"viewModel", "overrideViewModel"} {
		raw, exists := cfg[root]
		if !exists {
			continue
		}
		object, ok := raw.(map[string]interface{})
		if !ok {
			problems = append(problems, "data_config."+root+" 必须是对象，不能整体清除")
			continue
		}
		allowed := []string{"chartKind", "rules"}
		if root == "overrideViewModel" {
			allowed = []string{"cartesian"}
		}
		problems = append(problems, progressObjectFields(object, "data_config."+root, allowed...)...)
		problems = append(problems, validateNativeValues(object, "data_config."+root)...)
		if root == "viewModel" {
			if kind, exists := object["chartKind"]; exists && !isJSONNull(kind) {
				value, ok := kind.(float64)
				if !ok || value < 0 || math.Trunc(value) != value {
					problems = append(problems, "data_config.viewModel.chartKind 必须是非负整数或 null")
				}
			}
			if rawRules, exists := object["rules"]; exists {
				rules, ok := rawRules.(map[string]interface{})
				if !ok {
					problems = append(problems, "data_config.viewModel.rules 必须是对象，不能整体清除")
					continue
				}
				problems = append(problems, progressObjectFields(rules, "data_config.viewModel.rules", "plotArea", "theme", "legend", "series", "xyAxes", "formatInfo", "progress", "statistics", "pie", "radar", "area", "funnel", "wordClouds", "ranking", "nps", "countdown", "pivotTable")...)
				for node, value := range rules {
					if !isJSONNull(value) {
						if _, ok := value.(map[string]interface{}); !ok {
							problems = append(problems, "data_config.viewModel.rules."+node+" 必须是对象或 null")
						}
					}
				}
				if plotArea, ok := rules["plotArea"].(map[string]interface{}); ok {
					if raw, exists := plotArea["backgroundOpacity"]; exists && !isJSONNull(raw) {
						value, ok := raw.(float64)
						if !ok || value < 0 || value > 1 {
							problems = append(problems, "data_config.viewModel.rules.plotArea.backgroundOpacity 必须是 0 到 1 的数字或 null")
						}
					}
				}
				if format, ok := rules["formatInfo"].(map[string]interface{}); ok {
					problems = append(problems, validateNativeFormat(format, "data_config.viewModel.rules.formatInfo", true)...)
				}
				if stats, ok := rules["statistics"].(map[string]interface{}); ok {
					if format, exists := stats["formatInfo"]; exists && !isJSONNull(format) {
						if object, ok := format.(map[string]interface{}); ok {
							problems = append(problems, validateNativeFormat(object, "data_config.viewModel.rules.statistics.formatInfo", false)...)
						} else {
							problems = append(problems, "statistics.formatInfo 必须是对象或 null")
						}
					}
					if _, exists := stats["formatInfo"]; exists {
						_, legacy := cfg["number_format"]
						if multi, ok := cfg["multi_source"].(map[string]interface{}); ok {
							_, legacyMulti := multi["number_format"]
							legacy = legacy || legacyMulti
						}
						if legacy {
							problems = append(problems, "number_format 与原生 statistics.formatInfo 不能同时提交")
						}
					}
				}
			}
		}
	}
	return problems
}

func validateNativeFormat(format map[string]interface{}, path string, progress bool) []string {
	fields := []string{"formatName", "precision"}
	if progress {
		fields = append(fields, "percentPrecision", "unit", "unitType", "position")
	}
	problems := progressObjectFields(format, path, fields...)
	for _, field := range []string{"precision", "percentPrecision"} {
		if raw, exists := format[field]; exists && !isJSONNull(raw) {
			value, ok := raw.(float64)
			if !ok || math.Trunc(value) != value || value < 0 || value > 9 {
				problems = append(problems, path+"."+field+" 必须是 0 到 9 的整数或 null")
			}
		}
	}
	if raw, exists := format["formatName"]; exists && !isJSONNull(raw) {
		value, ok := raw.(string)
		if !ok || (value != "digital" && value != "digital_without_separator" && value != "scientific" && value != "percentage_rounded" && value != "cyn_rounded" && value != "dollar_rounded") {
			problems = append(problems, path+".formatName 不是支持的 Web 格式")
		}
	}
	return problems
}

func validateNativeValues(value interface{}, path string) []string {
	var problems []string
	switch object := value.(type) {
	case map[string]interface{}:
		for key, child := range object {
			if key == "__proto__" || key == "constructor" || key == "prototype" {
				problems = append(problems, path+" 包含非法 key")
			}
			problems = append(problems, validateNativeValues(child, path+"."+key)...)
		}
	case []interface{}:
		for i, child := range object {
			problems = append(problems, validateNativeValues(child, fmt.Sprintf("%s[%d]", path, i))...)
		}
	case float64:
		if math.IsNaN(object) || math.IsInf(object, 0) {
			problems = append(problems, path+" 必须是有限数字")
		}
	}
	return problems
}
