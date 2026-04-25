// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "strings"

// SanitizeAppID replaces ".." / path separators / NUL with "_" to guard filepath.Join; empty/dot-only collapses to "_".
func SanitizeAppID(appID string) string {
	if appID == "" {
		return "_"
	}
	repl := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		"\x00", "_",
		"..", "_",
	)
	out := repl.Replace(appID)
	if out == "" || out == "." {
		return "_"
	}
	return out
}
