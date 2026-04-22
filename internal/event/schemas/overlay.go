// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package schemas: overlay.go — apply FieldMeta decorations onto a parsed
// JSON Schema in place.
//
// Business authors declare language-level semantics two ways:
//  1. Struct tags (`desc:"..." enum:"..." kind:"..."`) — co-located with
//     the field, baked into the reflected schema by fromtype.go.
//  2. FieldOverrides map[pointer]FieldMeta on SchemaDef — an overlay
//     keyed by JSON Pointer paths, applied post-reflection (and
//     post-envelope for Native). Useful when:
//     - you can't edit the struct (SDK types)
//     - cross-nested-array paths are awkward to tag (/*/mime_type)
//     - you want to override a tag globally at registration time
//
// Overlay wins over tag. Paths that don't resolve in the target schema
// are returned as orphans; CI lint fails on orphans so typos / SDK drift
// can't silently mask schema claims.
package schemas

import "sort"

// FieldMeta is the semantic overlay for a single schema node. All fields
// are optional; non-empty fields replace the corresponding schema
// annotation.
type FieldMeta struct {
	Description string
	Enum        []string
	Kind        string // semantic marker (open_id / chat_id / email / timestamp_ms …); renders to JSON Schema "format" keyword
}

// ApplyFieldOverrides mutates `schema` in place, merging each entry of
// `overrides` onto the node(s) that its JSON Pointer path resolves to.
// Returns the list of pointer paths that resolved to nothing — callers
// (typically CI lint in tests) use this list to fail on orphans.
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
