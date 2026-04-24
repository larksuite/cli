// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "strings"

// SanitizeAppID guards filesystem path construction from a malformed
// AppID. Config corruption, manual edit, or a hostile caller could push
// "..", path separators, or NUL into the AppID string — using the raw
// value in filepath.Join would then escape the events/ subtree and let
// the caller place files anywhere under the config dir or its parents.
//
// Replace dangerous sequences with "_". "Wedged bus with an unreadable
// AppID directory" is acceptable; "bus.sock / bus.log landing under
// $HOME/.ssh/" is not. Applied everywhere AppID contributes to a path:
//
//   - internal/event/transport/transport_unix.go   (bus.sock)
//   - internal/event/transport/transport_windows.go (named pipe name)
//   - cmd/event/bus.go                              (bus.log directory)
//
// Empty or dot-only inputs collapse to "_" so filepath.Join never
// produces the config-dir root itself as the events/{appid} segment.
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
