// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeAppID_RejectsPathTraversal(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantClean   string
		forbidChars string
	}{
		{"happy path", "cli_XXXXXXXXXXXXXXXX", "cli_XXXXXXXXXXXXXXXX", "/\\\x00"},
		{"empty", "", "_", ""},
		{"dot", ".", "_", ""},
		{"double-dot only", "..", "_", ".."},
		{"leading traversal", "../etc/passwd", "__etc_passwd", "/"},
		{"traversal inside", "cli_../../etc", "cli_____etc", "/"},
		{"backslash traversal", "..\\windows\\system32", "__windows_system32", "\\"},
		{"nul injection", "cli_\x00backdoor", "cli__backdoor", "\x00"},
		{"pure slashes", "///", "___", "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeAppID(tc.input)
			if got != tc.wantClean {
				t.Errorf("SanitizeAppID(%q) = %q, want %q", tc.input, got, tc.wantClean)
			}
			for _, c := range tc.forbidChars {
				if strings.ContainsRune(got, c) {
					t.Errorf("SanitizeAppID(%q) = %q contains forbidden rune %q", tc.input, got, c)
				}
			}
			joined := filepath.ToSlash(filepath.Join("/root/events", got, "bus.log"))
			if strings.Contains(joined, "..") {
				t.Errorf("joined path %q contains .. after sanitization", joined)
			}
			if !strings.HasPrefix(joined, "/root/events/") {
				t.Errorf("joined path %q escaped /root/events/ parent", joined)
			}
		})
	}
}
