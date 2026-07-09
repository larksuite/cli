// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"
)

// TestRejectOutputTraversal pins HIGH-3: --output rejects absolute paths and
// any .. traversal component; empty and ordinary relative paths pass.
func TestRejectOutputTraversal(t *testing.T) {
	ok := []string{"", "out.csv", "./out.csv", "sub/dir/out.csv", "a..b.csv", "foo..bar/x.csv"}
	for _, p := range ok {
		if err := rejectOutputTraversal(p); err != nil {
			t.Errorf("rejectOutputTraversal(%q) = %v, want nil", p, err)
		}
	}
	bad := []string{"/etc/passwd", "../x", "../../etc/cron.d/evil", "sub/../../x", "./../x"}
	for _, p := range bad {
		if err := rejectOutputTraversal(p); err == nil {
			t.Errorf("rejectOutputTraversal(%q) = nil, want validation error", p)
		}
	}
}

// TestSanitizeUploadFileName pins HIGH-4 / LOW-1: control & separator chars are
// neutralized (percent-encoded, no raw CR/LF/TAB/NUL/quote) and the result never
// starts with a dot (hidden-file overwrite guard).
func TestSanitizeUploadFileName(t *testing.T) {
	// LOW-1: dotfiles get a leading underscore.
	for _, in := range []string{".bashrc", ".ssh", "..hidden"} {
		got := sanitizeUploadFileName(in)
		if strings.HasPrefix(got, ".") {
			t.Errorf("sanitizeUploadFileName(%q) = %q, must not start with '.'", in, got)
		}
	}
	// HIGH-4: header-breaking / control chars must not survive raw.
	raw := "a\r\nb\tc\x00d\"e.png"
	got := sanitizeUploadFileName(raw)
	for _, bad := range []string{"\r", "\n", "\t", "\x00", "\"", " "} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitizeUploadFileName(%q) = %q, still contains raw %q", raw, got, bad)
		}
	}
	if got == "" {
		t.Error("sanitizeUploadFileName returned empty for non-empty input")
	}
}
