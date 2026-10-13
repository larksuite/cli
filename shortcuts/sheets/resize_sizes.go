// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strconv"
	"strings"
)

// normalizeResizeSizesFlagValue folds observed list spellings into the
// existing size map. A positional list needs an exact A1 span; object entries
// must name their own target and contain no fields that would be discarded.
// Unrecognized shapes remain intact for the schema to reject.
func normalizeResizeSizesFlagValue(runtime flagView, value interface{}) interface{} {
	dimension := "column"
	if runtime.Command() == "+rows-resize" {
		dimension = "row"
	}
	if entries, ok := value.(map[string]interface{}); ok {
		for key, size := range entries {
			entries[key] = resizeNumericString(size)
		}
		return entries
	}
	list, ok := value.([]interface{})
	if !ok || len(list) == 0 {
		return value
	}
	entries := make(map[string]interface{}, len(list))
	if _, objects := list[0].(map[string]interface{}); objects {
		for _, raw := range list {
			item, ok := raw.(map[string]interface{})
			if !ok {
				return value
			}
			target, size, ok := resizeSizeEntry(item, dimension)
			if !ok {
				return value
			}
			if _, duplicate := entries[target]; duplicate {
				return value
			}
			entries[target] = resizeNumericString(size)
		}
		return entries
	}
	if !runtime.Changed("range") {
		return value
	}
	axis, start, end, err := parseA1Range(strings.TrimSpace(runtime.Str("range")))
	if err != nil || axis != dimension || end-start+1 != len(list) {
		return value
	}
	for i, size := range list {
		key := strconv.Itoa(start + i + 1)
		if dimension == "column" {
			key = columnIndexToLetter(start + i)
		}
		entries[key] = resizeNumericString(size)
	}
	return entries
}

func resizeSizeEntry(item map[string]interface{}, dimension string) (string, interface{}, bool) {
	var target string
	var size interface{}
	hasTarget, hasSize := false, false
	hasPixelMode := false
	for key, value := range item {
		switch {
		case key == "range" || (dimension == "row" && key == "row") || (dimension == "column" && (key == "column" || key == "col")):
			text, ok := value.(string)
			if !ok || hasTarget || strings.TrimSpace(text) == "" {
				return "", nil, false
			}
			target, hasTarget = strings.TrimSpace(text), true
		case key == "size" || key == pixelFlag(dimension):
			if hasSize {
				return "", nil, false
			}
			size, hasSize = value, true
		case key == "type" && value == "pixel":
			// Explicit pixel mode is redundant with the size field.
			hasPixelMode = true
		default:
			return "", nil, false
		}
	}
	if hasPixelMode {
		if _, number := resizeNumericString(size).(float64); !number {
			return "", nil, false
		}
	}
	return target, size, hasTarget && hasSize
}

// Pixel values have no text interpretation. Decode numeric strings with the
// same JSON number rules as inline numeric entries; modes stay strings.
func resizeNumericString(value interface{}) interface{} {
	if text, ok := value.(string); ok {
		var number float64
		if json.Unmarshal([]byte(strings.TrimSpace(text)), &number) == nil && strings.TrimSpace(text) != "null" {
			return number
		}
	}
	return value
}
