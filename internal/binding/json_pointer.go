// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"fmt"
	"strings"
)

// ReadJSONPointer navigates a parsed JSON value (typically the result of
// json.Unmarshal into interface{}) using an RFC 6901 JSON Pointer string.
//
// Supported pointer format: "/key/subkey/subsubkey".
// An empty pointer ("") returns data as-is.
// RFC 6901 escape sequences: ~1 → /, ~0 → ~.
//
// Limitation: only object (map) traversal is supported. Array index segments
// (e.g., "/channels/0/appId") are not implemented because OpenClaw's
// SecretRef file provider uses object-only paths in practice.
func ReadJSONPointer(data interface{}, pointer string) (interface{}, error) {
	if pointer == "" {
		return data, nil
	}

	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("json pointer must start with '/' or be empty, got %q", pointer)
	}

	// Split after the leading "/" and decode each segment.
	segments := strings.Split(pointer[1:], "/")
	current := data

	for i, raw := range segments {
		// RFC 6901 unescaping: ~1 → /, ~0 → ~ (order matters).
		key := strings.ReplaceAll(raw, "~1", "/")
		key = strings.ReplaceAll(key, "~0", "~")

		m, ok := current.(map[string]interface{})
		if !ok {
			traversed := "/" + strings.Join(segments[:i], "/")
			return nil, fmt.Errorf("json pointer %q: value at %q is %T, not an object",
				pointer, traversed, current)
		}

		val, exists := m[key]
		if !exists {
			return nil, fmt.Errorf("json pointer %q: key %q not found", pointer, key)
		}

		current = val
	}

	return current, nil
}
