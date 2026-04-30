// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schemas

import "sort"

// FieldMeta overrides win over struct tags; non-empty fields replace schema annotations.
type FieldMeta struct {
	Description string
	Enum        []string
	Kind        string // renders to JSON Schema "format" (open_id / chat_id / timestamp_ms …)
}

// ApplyFieldOverrides mutates schema in place; returns unresolved pointer paths (orphans).
func ApplyFieldOverrides(schema map[string]interface{}, overrides map[string]FieldMeta) []string {
	var orphans []string
	for path, meta := range overrides {
		nodes := ResolvePointer(schema, path)
		if len(nodes) == 0 {
			orphans = append(orphans, path)
			continue
		}
		for _, node := range nodes {
			if meta.Description != "" {
				node["description"] = meta.Description
			}
			if len(meta.Enum) > 0 {
				arr := make([]interface{}, len(meta.Enum))
				for i, v := range meta.Enum {
					arr[i] = v
				}
				node["enum"] = arr
			}
			if meta.Kind != "" {
				node["format"] = meta.Kind
			}
		}
	}
	sort.Strings(orphans)
	return orphans
}
