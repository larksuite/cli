// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package util

// TruncateStr truncates s to at most n runes, safe for multi-byte (e.g. CJK) characters.
func TruncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// TruncateStrWithEllipsis truncates s to at most n runes (including "..." suffix).
func TruncateStrWithEllipsis(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 3 {
		return string(r[:n])
	}
	return string(r[:n-3]) + "..."
}
