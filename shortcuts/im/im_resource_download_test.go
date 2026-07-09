// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import "testing"

// TestDownloadResourcePathSafety verifies the --download-resources path builder
// confines downloads to ./lark-im-resources/ and rejects abnormal file_keys
// (path separators, traversal, absolute paths) via the existing
// normalizeDownloadOutputPath guard (AC8).
func TestDownloadResourcePathSafety(t *testing.T) {
	if rel, err := resolveResourceDownloadPath("file_123"); err != nil || rel != "lark-im-resources/file_123" {
		t.Fatalf("resolveResourceDownloadPath(file_123) = (%q, %v), want (lark-im-resources/file_123, nil)", rel, err)
	}

	bad := []string{
		"",       // empty
		"a/b",    // forward slash
		`a\b`,    // backslash
		"..",     // traversal-only
		"../etc", // traversal with slash
		"/abs",   // absolute-ish (leading slash)
	}
	for _, key := range bad {
		if rel, err := resolveResourceDownloadPath(key); err == nil {
			t.Fatalf("resolveResourceDownloadPath(%q) = (%q, nil), want rejection", key, rel)
		}
	}
}
