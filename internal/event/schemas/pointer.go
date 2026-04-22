// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package schemas: pointer.go — resolve a JSON-Pointer-like path into a
// parsed JSON Schema tree, returning the matching sub-nodes so overlay
// can mutate them in place.
//
// Standard JSON Pointer (RFC 6901) navigates object properties; we extend
// with `/*` meaning "every element of an array", which resolves via the
// schema's `items` sub-tree. This matches business authors' mental model
// of "apply semantic overlay to each element of this list".
//
// The resolver returns []map[string]interface{} because callers mutate
// the node in place (adding description / enum / format). Paths with no
// match return an empty slice; callers treat this as silent skip (the
// CI lint pass is elsewhere and flags orphans).
package schemas

import "strings"

// ResolvePointer walks `schema` along the given path and returns any
// sub-nodes that match. Path syntax:
//
//   - "" or "/" → the root node itself
//   - "/foo" → property "foo" of the root object
//   - "/foo/bar" → nested property
//   - "/foo/*" → the items schema of an array property "foo"
//   - "/foo/*/bar" → property "bar" inside each element of array "foo"
//
// The function silently tolerates structural surprises (missing key,
// wrong type, non-array with `/*`) by returning a shorter slice — it is
// not validation.
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
				// Array element via `items`.
				items, ok := node["items"].(map[string]interface{})
				if !ok {
					continue
				}
				next = append(next, items)
				continue
			}
			// Property of an object.
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
