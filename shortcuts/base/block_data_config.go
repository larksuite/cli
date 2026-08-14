// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
)

// ── Public block types ───────────────────────────────────────────────

// chartBlockTypes are the chart and statistics block types shared by dashboard
// blocks and BaseApp page blocks.
var chartBlockTypes = []string{
	"column", "bar", "line", "pie", "ring", "scatter",
	"funnel", "wordCloud", "area", "combo", "radar", "statistics",
}

// textBlockTypes are the text-ish block types. Dashboard blocks and BaseApp
// page blocks share the same spelling "text" and the same data_config shape
// ({"text": "..."}).
var textBlockTypes = []string{"text"}

func matchesBlockType(blockType string, candidates []string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(blockType))
	for _, candidate := range candidates {
		if trimmed == strings.ToLower(candidate) {
			return true
		}
	}
	return false
}

func isTextBlockType(blockType string) bool { return matchesBlockType(blockType, textBlockTypes) }

func isChartBlockType(blockType string) bool { return matchesBlockType(blockType, chartBlockTypes) }

// appBlockTypes are all block types accepted by the BaseApp page block
// commands, in the order they appear in the protocol design.
func appBlockTypes() []string {
	types := make([]string, 0, len(chartBlockTypes)+2)
	types = append(types, chartBlockTypes...)
	types = append(types, "text")
	types = append(types, "list")
	return types
}

func isAppBlockType(blockType string) bool {
	return isChartBlockType(blockType) || matchesBlockType(blockType, []string{"text", "list"})
}

// ── data_config normalization & validation ───────────────────────────

// normalizeDataConfig normalizes data_config fields for chart blocks.
// It converts series[].rollup to uppercase and group_by[].sort fields to lowercase.
func normalizeDataConfig(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	out := cloneMap(cfg)
	// series[].rollup → 大写
	if arr, ok := out["series"].([]interface{}); ok {
		for i, it := range arr {
			if m, ok := it.(map[string]interface{}); ok {
				if r, ok := m["rollup"].(string); ok && r != "" {
					m["rollup"] = strings.ToUpper(strings.TrimSpace(r))
				}
				arr[i] = m
			}
		}
		out["series"] = arr
	}
	// group_by.sort 的 type/order → 小写
	if gb, ok := out["group_by"].([]interface{}); ok {
		for i, g := range gb {
			if m, ok := g.(map[string]interface{}); ok {
				if md, ok := m["mode"].(string); ok {
					m["mode"] = strings.ToLower(strings.TrimSpace(md))
				}
				if sub, ok := m["sort"].(map[string]interface{}); ok {
					sortType := ""
					if t, ok := sub["type"].(string); ok {
						sortType = strings.ToLower(strings.TrimSpace(t))
						sub["type"] = sortType
					}
					// Only lowercase a string order; leave a present-but-non-string
					// order untouched so validateBlockDataConfig can reject it
					// instead of it being silently coerced below.
					_, hasOrderKey := sub["order"]
					orderStr, orderIsString := sub["order"].(string)
					if orderIsString {
						sub["order"] = strings.ToLower(strings.TrimSpace(orderStr))
					}
					// Default only when the order key is truly absent. A present
					// key (even an illegal type/value) must survive to validation.
					if !hasOrderKey && (sortType == "group" || sortType == "view") {
						sub["order"] = "asc"
					}
					m["sort"] = sub
				}
				gb[i] = m
			}
		}
		out["group_by"] = gb
	}
	return out
}

// validateBlockDataConfig validates data_config based on block type.
// Text blocks only need a text field; everything else falls through to the
// dashboard chart rules. BaseApp list validation lives in
// app_list_block_data_config.go and never enters this dashboard path.
func validateBlockDataConfig(blockType string, cfg map[string]interface{}) []string {
	switch {
	case isTextBlockType(blockType):
		return validateTextDataConfig(blockType, cfg)
	default:
		return validateChartDataConfig(cfg)
	}
}

// validateTextDataConfig validates the text data_config shape.
func validateTextDataConfig(blockType string, cfg map[string]interface{}) []string {
	var problems []string
	if txt, _ := cfg["text"].(string); strings.TrimSpace(txt) == "" {
		problems = append(problems, fmt.Sprintf("%s 类型组件缺少必填字段 text", strings.TrimSpace(blockType)))
	}
	return problems
}

// validateChartDataConfig validates the chart/statistics data_config shape.
func validateChartDataConfig(cfg map[string]interface{}) []string {
	var errs []string

	// 图表类型通用校验
	// table_name 必填
	if tn, _ := cfg["table_name"].(string); strings.TrimSpace(tn) == "" {
		errs = append(errs, "缺少必填字段 table_name")
	}
	// series 与 count_all 互斥且必有其一
	_, hasSeries := cfg["series"]
	_, hasCountAll := cfg["count_all"]
	if !(hasSeries || hasCountAll) {
		errs = append(errs, "series 与 count_all 二选一，至少提供其一")
	}
	if hasSeries && hasCountAll {
		errs = append(errs, "series 与 count_all 互斥，不可同时存在")
	}
	// series 校验
	if hasSeries {
		arr, ok := cfg["series"].([]interface{})
		if !ok || len(arr) == 0 {
			errs = append(errs, "series 必须是非空数组")
		} else {
			// rollup 支持：SUM / MAX / MIN / AVERAGE（不支持 COUNTA；计数请使用 count_all）
			allowed := map[string]bool{"SUM": true, "MAX": true, "MIN": true, "AVERAGE": true}
			for i, it := range arr {
				m, ok := it.(map[string]interface{})
				if !ok {
					errs = append(errs, fmt.Sprintf("series[%d] 必须是对象", i))
					continue
				}
				fn, _ := m["field_name"].(string)
				if strings.TrimSpace(fn) == "" {
					errs = append(errs, fmt.Sprintf("series[%d].field_name 不能为空", i))
				}
				r, _ := m["rollup"].(string)
				r = strings.ToUpper(strings.TrimSpace(r))
				if !allowed[r] {
					errs = append(errs, fmt.Sprintf("series[%d].rollup 不在允许枚举内: %s", i, r))
				}
			}
		}
	}
	// group_by 最多 2 个，字段名必填，sort 合法
	if gb, ok := cfg["group_by"].([]interface{}); ok {
		if len(gb) > 2 {
			errs = append(errs, "group_by 最多支持 2 个维度")
		}
		for i, g := range gb {
			m, ok := g.(map[string]interface{})
			if !ok {
				errs = append(errs, fmt.Sprintf("group_by[%d] 必须是对象", i))
				continue
			}
			fn, _ := m["field_name"].(string)
			if strings.TrimSpace(fn) == "" {
				errs = append(errs, fmt.Sprintf("group_by[%d].field_name 不能为空", i))
			}
			if sub, ok := m["sort"].(map[string]interface{}); ok {
				t, _ := sub["type"].(string)
				t = strings.ToLower(strings.TrimSpace(t))
				if t != "group" && t != "value" && t != "view" {
					errs = append(errs, fmt.Sprintf("group_by[%d].sort.type 仅支持 group|value|view", i))
				}
				orderRaw, hasOrder := sub["order"]
				o, orderIsString := orderRaw.(string)
				o = strings.ToLower(strings.TrimSpace(o))
				switch {
				case !hasOrder:
					errs = append(errs, fmt.Sprintf("group_by[%d].sort.order 缺失；sort 存在时必须设置 order 为 asc 或 desc，例如 \"sort\":{\"type\":\"group\",\"order\":\"asc\"}", i))
				case !orderIsString || (o != "asc" && o != "desc"):
					errs = append(errs, fmt.Sprintf("group_by[%d].sort.order 仅支持 asc|desc", i))
				}
			}
		}
	}
	// filter 基本结构
	errs = append(errs, validateBlockFilter(cfg, "filter", false)...)
	return errs
}

// ── BaseApp chart data_config (multi-datasource) ─────────────────────
//
// BaseApp page charts differ from dashboard charts by supporting multiple
// data sources: base_token is a single top-level value shared by every
// source, while table_name/series/count_all/group_by/filter move into each data_sources[]
// element. The per-source value semantics are identical to the dashboard
// chart rules, so each element reuses normalizeDataConfig /
// validateChartDataConfig; the wrapper only adds the top-level structure.

// normalizeAppChartDataConfig normalizes each data_sources[] element with the
// shared chart normalization (series[].rollup upper-case, group_by[].sort
// lower-case). Top-level base_token/data_source_mode/sort pass through. It is
// also safe for partial updates and non-chart configs: when data_sources is
// absent the config is returned unchanged.
func normalizeAppChartDataConfig(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	out := cloneMap(cfg)
	if mode, ok := out["data_source_mode"].(string); ok {
		out["data_source_mode"] = strings.ToLower(strings.TrimSpace(mode))
	}
	if sortConfig, ok := out["sort"].(map[string]interface{}); ok {
		if sortType, ok := sortConfig["type"].(string); ok {
			sortConfig["type"] = strings.ToLower(strings.TrimSpace(sortType))
		}
		if order, ok := sortConfig["order"].(string); ok {
			sortConfig["order"] = strings.ToLower(strings.TrimSpace(order))
		}
		out["sort"] = sortConfig
	}
	if sources, ok := out["data_sources"].([]interface{}); ok {
		normalized := make([]interface{}, len(sources))
		for i, s := range sources {
			if m, ok := s.(map[string]interface{}); ok {
				normalized[i] = normalizeDataConfig(cloneMap(m))
			} else {
				normalized[i] = s
			}
		}
		out["data_sources"] = normalized
	}
	return out
}

// validateAppChartDataConfig validates the multi-datasource ChartDataConfig
// shape used by BaseApp page charts.
func validateAppChartDataConfig(blockType string, cfg map[string]interface{}) []string {
	var problems []string
	isStatistics := matchesBlockType(blockType, []string{"statistics"})
	allowed := map[string]bool{
		"base_token": true, "data_sources": true, "data_source_mode": true, "sort": true,
	}
	for key := range cfg {
		if !allowed[key] {
			problems = append(problems, fmt.Sprintf("图表 data_config 不支持字段 %s", key))
		}
	}

	// 顶层 base_token 必填；App 命令不带 --base-token，所有数据源共用它。
	if bt, _ := cfg["base_token"].(string); strings.TrimSpace(bt) == "" {
		problems = append(problems, "缺少必填字段 base_token；App 图表所有数据源共用顶层同一个 base_token")
	}

	// data_source_mode 可选枚举：aggregate（默认）/ compare。
	if mode, has := cfg["data_source_mode"]; has {
		s, _ := mode.(string)
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "aggregate", "compare":
		default:
			problems = append(problems, "data_source_mode 仅支持 aggregate|compare")
		}
	}

	// 顶层 sort：statistics 不允许；其余校验 type/order。
	if sortRaw, has := cfg["sort"]; has {
		switch {
		case isStatistics:
			problems = append(problems, "statistics 不允许配置顶层 sort")
		default:
			if sub, ok := sortRaw.(map[string]interface{}); ok {
				t, _ := sub["type"].(string)
				switch strings.ToLower(strings.TrimSpace(t)) {
				case "group", "value", "record":
				default:
					problems = append(problems, "sort.type 仅支持 group|value|record")
				}
				if o, hasOrder := sub["order"]; hasOrder {
					os, isString := o.(string)
					os = strings.ToLower(strings.TrimSpace(os))
					if !isString || (os != "asc" && os != "desc") {
						problems = append(problems, "sort.order 仅支持 asc|desc")
					}
				}
			} else {
				problems = append(problems, "sort 必须是对象")
			}
		}
	}

	// data_sources 必填、非空数组；每项复用图表通用校验。
	rawSources, has := cfg["data_sources"]
	if !has {
		return append(problems, "缺少必填字段 data_sources；至少提供一个数据源")
	}
	sources, ok := rawSources.([]interface{})
	if !ok || len(sources) == 0 {
		return append(problems, "data_sources 必须是至少包含一项的数组")
	}
	for i, s := range sources {
		m, ok := s.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("data_sources[%d] 必须是对象", i))
			continue
		}
		for _, p := range validateAppChartDataSourceConfig(m) {
			problems = append(problems, fmt.Sprintf("data_sources[%d]: %s", i, p))
		}
		// statistics 不配置分组。
		if isStatistics {
			if gb, ok := m["group_by"].([]interface{}); ok && len(gb) > 0 {
				problems = append(problems, fmt.Sprintf("data_sources[%d]: statistics 不允许配置 group_by", i))
			}
		}
	}
	return problems
}

// validateAppBlockDataConfig routes BaseApp block validation: text blocks use
// the shared text rule; chart/statistics blocks use the multi-datasource chart
// rule. List blocks validate in app_list_block_data_config.go and never reach
// here.
func validateAppBlockDataConfig(blockType string, cfg map[string]interface{}) []string {
	if isTextBlockType(blockType) {
		var problems []string
		for key := range cfg {
			if key != "text" {
				problems = append(problems, fmt.Sprintf("富文本 data_config 不支持字段 %s", key))
			}
		}
		if raw, exists := cfg["text"]; exists {
			if _, ok := raw.(string); !ok {
				problems = append(problems, "text 必须是字符串")
			}
		}
		return problems
	}
	return validateAppChartDataConfig(blockType, cfg)
}

func validateAppChartDataSourceConfig(cfg map[string]interface{}) []string {
	var problems []string
	allowed := map[string]bool{
		"table_name": true, "series": true, "count_all": true, "group_by": true, "filter": true,
	}
	for key := range cfg {
		if !allowed[key] {
			problems = append(problems, fmt.Sprintf("不支持字段 %s", key))
		}
	}
	if tableName, _ := cfg["table_name"].(string); strings.TrimSpace(tableName) == "" {
		problems = append(problems, "缺少必填字段 table_name")
	}
	seriesRaw, hasSeries := cfg["series"]
	countRaw, hasCountAll := cfg["count_all"]
	if hasSeries == hasCountAll {
		problems = append(problems, "series 与 count_all 必须二选一")
	}
	if hasSeries {
		series, ok := seriesRaw.([]interface{})
		if !ok || len(series) < 1 || len(series) > 20 {
			problems = append(problems, "series 必须是包含 1～20 项的数组")
		} else {
			allowedRollups := map[string]bool{"SUM": true, "MAX": true, "MIN": true, "AVERAGE": true}
			for i, raw := range series {
				item, ok := raw.(map[string]interface{})
				if !ok {
					problems = append(problems, fmt.Sprintf("series[%d] 必须是对象", i))
					continue
				}
				if fieldName, _ := item["field_name"].(string); strings.TrimSpace(fieldName) == "" {
					problems = append(problems, fmt.Sprintf("series[%d].field_name 必填", i))
				}
				rollup, _ := item["rollup"].(string)
				if !allowedRollups[rollup] {
					problems = append(problems, fmt.Sprintf("series[%d].rollup 仅支持 SUM|MAX|MIN|AVERAGE", i))
				}
			}
		}
	}
	if hasCountAll {
		if count, ok := countRaw.(bool); !ok || !count {
			problems = append(problems, "count_all 只允许传 true")
		}
	}
	if groupsRaw, exists := cfg["group_by"]; exists {
		groups, ok := groupsRaw.([]interface{})
		if !ok || len(groups) > 2 {
			problems = append(problems, "group_by 必须是最多包含 2 项的数组")
		} else {
			for i, raw := range groups {
				group, ok := raw.(map[string]interface{})
				if !ok {
					problems = append(problems, fmt.Sprintf("group_by[%d] 必须是对象", i))
					continue
				}
				if fieldName, _ := group["field_name"].(string); strings.TrimSpace(fieldName) == "" {
					problems = append(problems, fmt.Sprintf("group_by[%d].field_name 必填", i))
				}
				if modeRaw, exists := group["mode"]; exists {
					mode, _ := modeRaw.(string)
					if mode != "enumerated" && mode != "integrated" {
						problems = append(problems, fmt.Sprintf("group_by[%d].mode 仅支持 enumerated|integrated", i))
					}
				}
				if sortRaw, exists := group["sort"]; exists {
					sortConfig, ok := sortRaw.(map[string]interface{})
					if !ok {
						problems = append(problems, fmt.Sprintf("group_by[%d].sort 必须是对象", i))
						continue
					}
					sortType, _ := sortConfig["type"].(string)
					if sortType != "group" && sortType != "value" && sortType != "view" {
						problems = append(problems, fmt.Sprintf("group_by[%d].sort.type 仅支持 group|value|view", i))
					}
					if orderRaw, exists := sortConfig["order"]; exists {
						order, _ := orderRaw.(string)
						if order != "asc" && order != "desc" {
							problems = append(problems, fmt.Sprintf("group_by[%d].sort.order 仅支持 asc|desc", i))
						}
					}
				}
			}
		}
	}
	problems = append(problems, validateProtocolFilter(cfg, "filter")...)
	return problems
}

func validateProtocolFilter(cfg map[string]interface{}, key string) []string {
	raw, exists := cfg[key]
	if !exists {
		return nil
	}
	filter, ok := raw.(map[string]interface{})
	if !ok {
		return []string{key + " 必须是对象"}
	}
	var problems []string
	conjunction, _ := filter["conjunction"].(string)
	if conjunction != "and" && conjunction != "or" {
		problems = append(problems, key+".conjunction 必填且仅支持 and|or")
	}
	conditions, ok := filter["conditions"].([]interface{})
	if !ok || len(conditions) < 1 || len(conditions) > 50 {
		return append(problems, key+".conditions 必须是包含 1～50 项的数组")
	}
	allowedOperators := map[string]bool{
		"is": true, "isNot": true, "contains": true, "doesNotContain": true,
		"isEmpty": true, "isNotEmpty": true, "isGreater": true, "isGreaterEqual": true,
		"isLess": true, "isLessEqual": true,
	}
	for i, rawCondition := range conditions {
		condition, ok := rawCondition.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("%s.conditions[%d] 必须是对象", key, i))
			continue
		}
		if fieldName, _ := condition["field_name"].(string); strings.TrimSpace(fieldName) == "" {
			problems = append(problems, fmt.Sprintf("%s.conditions[%d].field_name 必填", key, i))
		}
		operator, _ := condition["operator"].(string)
		if !allowedOperators[operator] {
			problems = append(problems, fmt.Sprintf("%s.conditions[%d].operator 不支持: %s", key, i, operator))
		}
		value, hasValue := condition["value"]
		if operator != "isEmpty" && operator != "isNotEmpty" && !hasValue {
			problems = append(problems, fmt.Sprintf("%s.conditions[%d].value 缺失", key, i))
		}
		if hasValue && !validProtocolFilterValue(value) {
			problems = append(problems, fmt.Sprintf("%s.conditions[%d].value 必须是字符串、数字、布尔值或最多 200 项的对应数组", key, i))
		}
	}
	return problems
}

func validProtocolFilterValue(value interface{}) bool {
	switch typed := value.(type) {
	case string, float64, bool, json.Number:
		return true
	case []interface{}:
		if len(typed) > 200 {
			return false
		}
		for _, item := range typed {
			switch item.(type) {
			case string, float64, bool, json.Number:
			default:
				return false
			}
		}
		return true
	default:
		return false
	}
}

// validateBlockFilter validates the filter object shared by chart and list
// data_config. key is the config key holding the filter ("filter").
// allowFieldID lets list blocks reference a field by ID; chart blocks keep the
// dashboard rule of field_name only.
func validateBlockFilter(cfg map[string]interface{}, key string, allowFieldID bool) []string {
	f, ok := cfg[key].(map[string]interface{})
	if !ok {
		return nil
	}
	var problems []string
	conj := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", f["conjunction"])))
	if conj == "" {
		conj = "and"
	}
	if conj != "and" && conj != "or" {
		problems = append(problems, key+".conjunction 仅支持 and|or")
	}
	conds, ok := f["conditions"].([]interface{})
	if !ok {
		return problems
	}
	allowedOps := map[string]bool{"is": true, "isnot": true, "contains": true, "doesnotcontain": true, "isempty": true, "isnotempty": true, "isgreater": true, "isgreaterequal": true, "isless": true, "islessequal": true}
	for i, it := range conds {
		m, ok := it.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("%s.conditions[%d] 必须是对象", key, i))
			continue
		}
		fn, _ := m["field_name"].(string)
		hasRef := strings.TrimSpace(fn) != ""
		if !hasRef && allowFieldID {
			fid, _ := m["field_id"].(string)
			hasRef = strings.TrimSpace(fid) != ""
		}
		if !hasRef {
			problems = append(problems, fmt.Sprintf("%s.conditions[%d].field_name 不能为空", key, i))
		}
		op, _ := m["operator"].(string)
		opKey := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(op), " ", ""))
		if !allowedOps[opKey] {
			problems = append(problems, fmt.Sprintf("%s.conditions[%d].operator 不支持: %s", key, i, op))
		}
		if opKey != "isempty" && opKey != "isnotempty" {
			if _, has := m["value"]; !has {
				problems = append(problems, fmt.Sprintf("%s.conditions[%d].value 缺失", key, i))
			}
		}
	}
	return problems
}

func formatDataConfigErrors(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "data_config 校验失败:\n- %s\n参考: skills/lark-base/references/lark-base-dashboard-block-config.md（应用页面组件见 lark-base-app-block-data-config.md）", strings.Join(problems, "\n- ")).WithParam("--data-config")
}
