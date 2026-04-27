// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar_demo

package main

import "strings"

// sanitizePath strips query parameters and replaces ID-like path segments
// with ":id" to prevent document tokens, chat IDs, etc. from leaking into logs.
// Example: /open-apis/docx/v1/documents/doxcnXXXX/blocks → /open-apis/docx/v1/documents/:id/blocks
func sanitizePath(pathAndQuery string) string {
	// Strip query
	path := pathAndQuery
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	// Replace ID-like segments (8+ chars, not a pure API keyword)
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if looksLikeID(p) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

// looksLikeID returns true if a path segment appears to be a resource identifier
// rather than an API route keyword. Heuristic: 8+ chars and contains a digit.
func looksLikeID(seg string) bool {
	if len(seg) < 8 {
		return false
	}
	for _, c := range seg {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

// sanitizeError returns a safe error string for logging, capped at 200 bytes
// to avoid dumping upstream response bodies into audit logs.
func sanitizeError(err error) string {
	s := err.Error()
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
