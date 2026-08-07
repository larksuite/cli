// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"strings"
)

var appListSubTypes = []string{"standard", "grouped", "collapsible", "card", "detail"}

func normalizeAppListSubType(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "standard", true
	}
	for _, candidate := range appListSubTypes {
		if value == candidate {
			return candidate, true
		}
	}
	return value, false
}

// validateAppListDataConfig validates only the published protocol shape. It
// intentionally does not infer UI semantics such as title/group_by cardinality
// or field roles.
func validateAppListDataConfig(subType string, cfg map[string]interface{}) []string {
	var problems []string
	allowed := map[string]bool{
		"base_token": true, "table_name": true, "filter": true, "sort_by": true,
	}
	switch subType {
	case "standard", "grouped", "collapsible":
		allowed["columns"] = true
		allowed["group_by"] = true
	case "card":
		allowed["fields"] = true
		allowed["card_config"] = true
	case "detail":
		allowed["fields"] = true
		allowed["detail_config"] = true
	}
	for key := range cfg {
		if !allowed[key] {
			problems = append(problems, fmt.Sprintf("%s 列表不支持字段 %s", subType, key))
		}
	}
	if token, _ := cfg["base_token"].(string); strings.TrimSpace(token) == "" {
		problems = append(problems, "缺少必填字段 base_token")
	}
	if tableName, _ := cfg["table_name"].(string); strings.TrimSpace(tableName) == "" {
		problems = append(problems, "缺少必填字段 table_name")
	}
	for _, key := range []string{"columns", "fields", "group_by", "sort_by"} {
		if raw, ok := cfg[key]; ok {
			if _, isArray := raw.([]interface{}); !isArray {
				problems = append(problems, key+" 必须是数组")
			}
		}
	}
	for _, key := range []string{"filter", "card_config", "detail_config"} {
		if raw, ok := cfg[key]; ok {
			if _, isObject := raw.(map[string]interface{}); !isObject {
				problems = append(problems, key+" 必须是对象")
			}
		}
	}
	problems = append(problems, validateListFilter(cfg)...)
	problems = append(problems, validateListNamedItems(cfg, "sort_by")...)
	problems = append(problems, validateListNamedItems(cfg, "group_by")...)
	problems = append(problems, validateListColumns(cfg)...)
	problems = append(problems, validateListStringFields(cfg)...)
	problems = append(problems, validateListOptionalStringConfig(cfg, "card_config", "title_field_name", "image_field_name")...)
	problems = append(problems, validateListOptionalStringConfig(cfg, "detail_config", "image_field_name")...)
	return problems
}

func validateListFilter(cfg map[string]interface{}) []string {
	return validateProtocolFilter(cfg, "filter")
}

func validateListNamedItems(cfg map[string]interface{}, key string) []string {
	items, ok := cfg[key].([]interface{})
	if !ok {
		return nil
	}
	var problems []string
	for i, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("%s[%d] 必须是对象", key, i))
			continue
		}
		if value, _ := item["field_name"].(string); strings.TrimSpace(value) == "" {
			problems = append(problems, fmt.Sprintf("%s[%d].field_name 必填", key, i))
		}
		if order, exists := item["order"]; exists {
			value, _ := order.(string)
			if value != "asc" && value != "desc" {
				problems = append(problems, fmt.Sprintf("%s[%d].order 仅支持 asc|desc", key, i))
			}
		}
	}
	return problems
}

func validateListColumns(cfg map[string]interface{}) []string {
	items, ok := cfg["columns"].([]interface{})
	if !ok {
		return nil
	}
	var problems []string
	for i, raw := range items {
		column, ok := raw.(map[string]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("columns[%d] 必须是对象", i))
			continue
		}
		columnType, _ := column["type"].(string)
		switch columnType {
		case "field":
			if value, _ := column["field_name"].(string); strings.TrimSpace(value) == "" {
				problems = append(problems, fmt.Sprintf("columns[%d].field_name 必填", i))
			}
		case "combined":
			fieldNames, ok := column["field_names"].([]interface{})
			if !ok || len(fieldNames) == 0 {
				problems = append(problems, fmt.Sprintf("columns[%d].field_names 必须至少包含一个字段", i))
				continue
			}
			for j, rawName := range fieldNames {
				if _, ok := rawName.(string); !ok {
					problems = append(problems, fmt.Sprintf("columns[%d].field_names[%d] 必须是字符串", i, j))
				}
			}
		default:
			problems = append(problems, fmt.Sprintf("columns[%d].type 必填且仅支持 field|combined", i))
		}
	}
	return problems
}

func validateListStringFields(cfg map[string]interface{}) []string {
	items, ok := cfg["fields"].([]interface{})
	if !ok {
		return nil
	}
	var problems []string
	for i, raw := range items {
		if _, ok := raw.(string); !ok {
			problems = append(problems, fmt.Sprintf("fields[%d] 必须是字符串", i))
		}
	}
	return problems
}

func validateListOptionalStringConfig(cfg map[string]interface{}, key string, fields ...string) []string {
	obj, ok := cfg[key].(map[string]interface{})
	if !ok {
		return nil
	}
	var problems []string
	for _, field := range fields {
		if raw, exists := obj[field]; exists {
			if _, ok := raw.(string); !ok {
				problems = append(problems, key+"."+field+" 必须是字符串")
			}
		}
	}
	return problems
}
