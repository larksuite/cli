// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schemas

import "strings"

// ResolvePointer extends RFC 6901 with `/*` for array items; tolerates structural mismatches.
func ResolvePointer(schema map[string]interface{}, path string) []map[string]interface{} {
	if path == "" || path == "/" {
		return []map[string]interface{}{schema}
	}
	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.Split(trimmed, "/")

	current := []map[string]interface{}{schema}
	for _, part := range parts {
		next := []map[string]interface{}{}
		for _, node := range current {
			if part == "*" {
				items, ok := node["items"].(map[string]interface{})
				if !ok {
					continue
				}
				next = append(next, items)
				continue
			}
			props, ok := node["properties"].(map[string]interface{})
			if !ok {
				continue
			}
			child, ok := props[part].(map[string]interface{})
			if !ok {
				continue
			}
			next = append(next, child)
		}
		if len(next) == 0 {
			return nil
		}
		current = next
	}
	return current
}
