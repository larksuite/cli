// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "github.com/larksuite/cli/internal/util"

// GetString safely extracts a string from a nested map path.
// Usage: GetString(data, "user", "name") is equivalent to
// data["user"].(map[string]interface{})["name"].(string)
func GetString(m map[string]interface{}, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	v := navigate(m, keys[:len(keys)-1])
	if v == nil {
		return ""
	}
	s, _ := v[keys[len(keys)-1]].(string)
	return s
}

// GetFloat safely extracts a float64 (the default JSON number type).
func GetFloat(m map[string]interface{}, keys ...string) float64 {
	f, _ := GetFloatOK(m, keys...)
	return f
}

// GetFloatOK extracts a float64 and reports whether the field was present and
// numeric. Use it for protocol discriminators where silently turning malformed
// input into zero could misclassify a response as successful.
func GetFloatOK(m map[string]interface{}, keys ...string) (float64, bool) {
	if len(keys) == 0 {
		return 0, false
	}
	v := navigate(m, keys[:len(keys)-1])
	if v == nil {
		return 0, false
	}
	f, ok := util.ToFloat64(v[keys[len(keys)-1]])
	return f, ok
}

// GetInt safely extracts an int, accepting both in-memory ints and JSON-style float64 values.
func GetInt(m map[string]interface{}, keys ...string) int {
	if len(keys) == 0 {
		return 0
	}
	v := navigate(m, keys[:len(keys)-1])
	if v == nil {
		return 0
	}
	switch n := v[keys[len(keys)-1]].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// GetBool safely extracts a bool.
func GetBool(m map[string]interface{}, keys ...string) bool {
	if len(keys) == 0 {
		return false
	}
	v := navigate(m, keys[:len(keys)-1])
	if v == nil {
		return false
	}
	b, _ := v[keys[len(keys)-1]].(bool)
	return b
}

// GetMap safely extracts a nested map.
func GetMap(m map[string]interface{}, keys ...string) map[string]interface{} {
	if len(keys) == 0 {
		return m
	}
	return navigate(m, keys)
}

// GetSlice safely extracts a []interface{}.
func GetSlice(m map[string]interface{}, keys ...string) []interface{} {
	if len(keys) == 0 {
		return nil
	}
	v := navigate(m, keys[:len(keys)-1])
	if v == nil {
		return nil
	}
	s, _ := v[keys[len(keys)-1]].([]interface{})
	return s
}

// EachMap iterates over map elements in a slice, skipping non-map items.
func EachMap(items []interface{}, fn func(m map[string]interface{})) {
	if fn == nil {
		return
	}
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			fn(m)
		}
	}
}

// navigate walks a map along the given keys, returning nil if any step fails.
func navigate(m map[string]interface{}, keys []string) map[string]interface{} {
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]interface{})
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}
