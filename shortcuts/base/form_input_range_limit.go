// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// input_range_limit 是题目级的填写范围约束，结构由 type 判别，三类互不共享参数。
// 这里做结构校验而不是透传，是因为写入成功的响应不回显 input_range_limit：
// 字段名或类型写错时服务端可能静默忽略，命令仍返回成功，模型无从察觉。
//
// 校验口径对齐下游 node_product 的 zod 契约（service/openapi/form/schema.ts）：
// 宽松于下游会放过注定失败的请求，严于下游会拦掉本可成功的写法，两者都不可接受。
const (
	inputRangeLimitTextLength  = "text_length"
	inputRangeLimitNumberRange = "number_range"
	inputRangeLimitDateRange   = "date_range"

	// textLengthMaxBound 对齐下游 FORM_TEXT_LENGTH_MAX。
	textLengthMaxBound = 100000
)

// inputRangeLimitTypes 是 CLI 接受的限制类型，用于枚举归一化与报错；不在其中的
// type 在本地拒绝，不让注定失败的写法发出去。
var inputRangeLimitTypes = []string{
	inputRangeLimitTextLength, inputRangeLimitNumberRange,
	inputRangeLimitDateRange,
}

// inputRangeLimitFields 是各 type 允许出现的字段白名单，对应下游各分支的 .strict()。
// 驼峰字段名（minLength）和跨类型串用（text_length 里写 min_value）都在这里被挡下。
var inputRangeLimitFields = map[string]map[string]bool{
	inputRangeLimitTextLength:  {"type": true, "min_length": true, "max_length": true},
	inputRangeLimitNumberRange: {"type": true, "min_value": true, "max_value": true},
	inputRangeLimitDateRange:   {"type": true, "earliest": true, "latest": true},
}

// dateRangeAnchors 相对日期锚点全集，顺序与下游 DateAnchor 一致。
// 协议里没有「7 天前」这类具名枚举，只能由 anchor 搭配 offset_days 组合。
var dateRangeAnchors = []string{
	"today", "monday", "friday", "saturday", "sunday", "month_start", "month_end",
}

// normalizeEnumAlias 复刻下游 filter-schema.ts 的 normalizeEnum：按固定的别名变体
// 集合匹配，而不是做任意大小写折叠。少认一种变体就会拦掉下游本可接受的写法。
// 归一化只用于校验，请求体仍原样透传。
func normalizeEnumAlias(value string, allowed []string) string {
	input := strings.TrimSpace(value)
	for _, key := range allowed {
		parts := strings.Split(key, "_")
		capitalized := make([]string, len(parts))
		for i, part := range parts {
			if part == "" {
				continue
			}
			capitalized[i] = strings.ToUpper(part[:1]) + part[1:]
		}
		variants := []string{
			key,
			strings.ToUpper(key),
			strings.Join(parts, "-"),
			strings.ToUpper(strings.Join(parts, "-")),
			strings.Join(parts, " "),
			strings.Join(capitalized, " "),
			parts[0] + strings.Join(capitalized[1:], ""),
			strings.Join(capitalized, ""),
		}
		if slices.Contains(variants, input) {
			return key
		}
	}
	return value
}

// validateInputRangeLimit 校验单个题目的 input_range_limit。
// raw 为 nil（显式传 null）表示清除限制，属于合法输入。
func validateInputRangeLimit(path string, raw interface{}) error {
	var limit map[string]interface{}
	switch v := raw.(type) {
	case nil:
		return nil
	case map[string]interface{}:
		limit = v
	default:
		return baseValidationErrorf("%s.input_range_limit must be an object or null", path)
	}
	limitPath := path + ".input_range_limit"

	rawType, ok := limit["type"].(string)
	if !ok || strings.TrimSpace(rawType) == "" {
		return baseValidationErrorf("%s.type must be a non-empty string", limitPath)
	}
	limitType := normalizeEnumAlias(rawType, inputRangeLimitTypes)
	allowed, ok := inputRangeLimitFields[limitType]
	if !ok {
		return baseValidationErrorf("%s.type must be one of: %s", limitPath, strings.Join(inputRangeLimitTypes, ", "))
	}
	// 按字段名排序后再检查：同时有多个非法字段时，报出的总是同一个。
	for _, name := range slices.Sorted(maps.Keys(limit)) {
		if !allowed[name] {
			return baseValidationErrorf("%s.%s is not a valid field for type %q; allowed: %s",
				limitPath, name, rawType, sortedFieldNames(allowed))
		}
	}

	switch limitType {
	case inputRangeLimitTextLength:
		return validateTextLengthLimit(limitPath, limit)
	case inputRangeLimitNumberRange:
		return validateNumberRangeLimit(limitPath, limit)
	default:
		return validateDateRangeLimit(limitPath, limit)
	}
}

// validateTextLengthLimit 字数区间。下游是 z.number().int().positive()，
// 即严格正整数：0 会被拒绝，不是「非负」。
func validateTextLengthLimit(path string, limit map[string]interface{}) error {
	minVal, hasMin, err := optionalJSONNumber(path, limit, "min_length")
	if err != nil {
		return err
	}
	maxVal, hasMax, err := optionalJSONNumber(path, limit, "max_length")
	if err != nil {
		return err
	}
	if !hasMin && !hasMax {
		return baseValidationErrorf("%s must set min_length or max_length", path)
	}
	if hasMin && !isPositiveInteger(minVal) {
		return baseValidationErrorf("%s.min_length must be a positive integer", path)
	}
	if hasMax && !isPositiveInteger(maxVal) {
		return baseValidationErrorf("%s.max_length must be a positive integer", path)
	}
	if hasMax && maxVal > textLengthMaxBound {
		return baseValidationErrorf("%s.max_length must not exceed %d", path, textLengthMaxBound)
	}
	if hasMin && hasMax && minVal > maxVal {
		return baseValidationErrorf("%s.min_length must not exceed max_length", path)
	}
	return nil
}

// validateNumberRangeLimit 数值区间：允许 0、负数和小数，至少给一个边界，给全时 min <= max。
func validateNumberRangeLimit(path string, limit map[string]interface{}) error {
	minVal, hasMin, err := optionalJSONNumber(path, limit, "min_value")
	if err != nil {
		return err
	}
	maxVal, hasMax, err := optionalJSONNumber(path, limit, "max_value")
	if err != nil {
		return err
	}
	if !hasMin && !hasMax {
		return baseValidationErrorf("%s must set min_value or max_value", path)
	}
	if hasMin && hasMax && minVal > maxVal {
		return baseValidationErrorf("%s.min_value must not exceed max_value", path)
	}
	return nil
}

// validateDateRangeLimit 日期区间：至少给一个边界；两端都用 fixed_ms 时 earliest 不得晚于 latest
// （下游只在两端同为 fixed_ms 时比较，dynamic 的先后要到运行期才知道）。
func validateDateRangeLimit(path string, limit map[string]interface{}) error {
	fixed := map[string]float64{}
	hasAny := false
	for _, bound := range []string{"earliest", "latest"} {
		raw, ok := limit[bound]
		if !ok {
			continue
		}
		hasAny = true
		fixedMS, hasFixed, err := validateDateRangeBound(path+"."+bound, raw)
		if err != nil {
			return err
		}
		if hasFixed {
			fixed[bound] = fixedMS
		}
	}
	if !hasAny {
		return baseValidationErrorf("%s must set earliest or latest", path)
	}
	earliest, hasEarliest := fixed["earliest"]
	latest, hasLatest := fixed["latest"]
	if hasEarliest && hasLatest && earliest > latest {
		return baseValidationErrorf("%s.earliest must not exceed latest", path)
	}
	return nil
}

// validateDateRangeBound 单个日期边界：fixed_ms 与 dynamic 互斥且必须二选一。
// 返回 fixed_ms 的值供上层做先后比较。
func validateDateRangeBound(path string, raw interface{}) (float64, bool, error) {
	bound, ok := raw.(map[string]interface{})
	if !ok {
		return 0, false, baseValidationErrorf("%s must be an object", path)
	}
	_, hasFixed := bound["fixed_ms"]
	dynamicRaw, hasDynamic := bound["dynamic"]
	for name := range bound {
		if name != "fixed_ms" && name != "dynamic" {
			return 0, false, baseValidationErrorf("%s.%s is not a valid field; allowed: dynamic, fixed_ms", path, name)
		}
	}
	if hasFixed && hasDynamic {
		return 0, false, baseValidationErrorf("%s must set either fixed_ms or dynamic, not both", path)
	}
	if !hasFixed && !hasDynamic {
		return 0, false, baseValidationErrorf("%s must set fixed_ms or dynamic", path)
	}
	if hasFixed {
		fixedMS, _, err := optionalJSONNumber(path, bound, "fixed_ms")
		if err != nil {
			return 0, false, err
		}
		if fixedMS != float64(int64(fixedMS)) {
			return 0, false, baseValidationErrorf("%s.fixed_ms must be an integer number of milliseconds", path)
		}
		return fixedMS, true, nil
	}
	return 0, false, validateDateRangeDynamic(path+".dynamic", dynamicRaw)
}

// validateDateRangeDynamic 相对日期：anchor 取自固定枚举，offset_days 是整数天，可正可负。
func validateDateRangeDynamic(path string, raw interface{}) error {
	dynamic, ok := raw.(map[string]interface{})
	if !ok {
		return baseValidationErrorf("%s must be an object", path)
	}
	for name := range dynamic {
		if name != "anchor" && name != "offset_days" {
			return baseValidationErrorf("%s.%s is not a valid field; allowed: anchor, offset_days", path, name)
		}
	}
	anchor, ok := dynamic["anchor"].(string)
	if !ok || strings.TrimSpace(anchor) == "" {
		return baseValidationErrorf("%s.anchor must be a non-empty string", path)
	}
	if !slices.Contains(dateRangeAnchors, normalizeEnumAlias(anchor, dateRangeAnchors)) {
		return baseValidationErrorf("%s.anchor must be one of: %s", path, strings.Join(dateRangeAnchors, ", "))
	}
	if _, ok := dynamic["offset_days"]; ok {
		offset, _, err := optionalJSONNumber(path, dynamic, "offset_days")
		if err != nil {
			return err
		}
		if offset != float64(int64(offset)) {
			return baseValidationErrorf("%s.offset_days must be an integer number of days", path)
		}
	}
	return nil
}

// isPositiveInteger 判断是否为严格大于 0 的整数。
func isPositiveInteger(value float64) bool {
	return value > 0 && value == float64(int64(value))
}

// optionalJSONNumber 读取可选数值字段。字符串形态（"10"）在这里被判为非法，
// 因为协议要求 JSON number，字符串会被服务端按未知类型处理。
func optionalJSONNumber(path string, obj map[string]interface{}, name string) (float64, bool, error) {
	raw, ok := obj[name]
	if !ok {
		return 0, false, nil
	}
	value, ok := raw.(float64)
	if !ok {
		return 0, false, baseValidationErrorf("%s.%s must be a JSON number, not %s", path, name, jsonTypeName(raw))
	}
	return value, true, nil
}

// jsonTypeName 把实际收到的 JSON 类型翻译成给模型看的名字，便于定位「把数字写成字符串」这类错误。
func jsonTypeName(raw interface{}) string {
	switch raw.(type) {
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case []interface{}:
		return "an array"
	case map[string]interface{}:
		return "an object"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", raw)
	}
}

// sortedFieldNames 输出稳定顺序的字段名列表，保证错误信息可断言。
func sortedFieldNames(set map[string]bool) string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}
