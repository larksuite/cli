// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"strings"
)

// Only create may default the display configuration, and only when all three
// keys are absent. Explicit nulls and partial configurations must reach validation.
func normalizeSlicerDataConfigForCreate(cfg map[string]interface{}) map[string]interface{} {
	out := cloneMap(cfg)
	if out == nil {
		return nil
	}
	_, hasDisplay := out["displayMode"]
	_, hasSelect := out["selectMode"]
	_, hasOrder := out["order"]
	if !hasDisplay && !hasSelect && !hasOrder {
		out["displayMode"] = "tiled"
		out["selectMode"] = "multiple"
		out["order"] = "asc"
	}
	return out
}

// Slicers expose one grouping field and no metric. Keep this separate from the
// chart validator and shared chartBlockTypes, which also admit BaseApp types.
func validateSlicerDataConfig(cfg map[string]interface{}) []string {
	var problems []string
	for _, field := range unexpectedObjectFields(cfg, "table_name", "group_by", "filter", "displayMode", "selectMode", "order") {
		problems = append(problems, fmt.Sprintf("slicer 不支持字段 %s", field))
	}
	if tableName, _ := cfg["table_name"].(string); strings.TrimSpace(tableName) == "" {
		problems = append(problems, "slicer 缺少必填字段 table_name")
	}

	groups, ok := cfg["group_by"].([]interface{})
	if !ok || len(groups) != 1 {
		problems = append(problems, "slicer.group_by 必须是恰好包含 1 个字段的数组")
	} else if group, ok := groups[0].(map[string]interface{}); !ok {
		problems = append(problems, "slicer.group_by[0] 必须是对象")
	} else {
		for _, field := range unexpectedObjectFields(group, "field_name", "mode") {
			problems = append(problems, fmt.Sprintf("slicer.group_by[0] 不支持字段 %s", field))
		}
		if fieldName, _ := group["field_name"].(string); strings.TrimSpace(fieldName) == "" {
			problems = append(problems, "slicer.group_by[0].field_name 不能为空")
		}
		if rawMode, exists := group["mode"]; exists {
			if mode, ok := rawMode.(string); !ok || (mode != "integrated" && mode != "enumerated") {
				problems = append(problems, "slicer.group_by[0].mode 仅支持 integrated|enumerated")
			}
		}
	}

	displayFieldCount := 0
	for _, field := range []struct {
		name    string
		allowed [2]string
	}{
		{"displayMode", [2]string{"dropdown", "tiled"}},
		{"selectMode", [2]string{"single", "multiple"}},
		{"order", [2]string{"asc", "desc"}},
	} {
		if raw, exists := cfg[field.name]; exists {
			displayFieldCount++
			value, ok := raw.(string)
			if !ok || (value != field.allowed[0] && value != field.allowed[1]) {
				problems = append(problems, fmt.Sprintf("slicer.%s 仅支持 %s", field.name, strings.Join(field.allowed[:], "|")))
			}
		}
	}
	if displayFieldCount != 0 && displayFieldCount != 3 {
		problems = append(problems, "slicer.displayMode、selectMode、order 必须同时提供或同时省略")
	}
	return append(problems, validateProtocolFilter(cfg, "filter")...)
}
