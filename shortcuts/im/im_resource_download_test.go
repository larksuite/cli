// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/validate"
)

// TestDownloadOutputReachesThePathPolicy pins the split this command relies on:
// --output only has to name something, and the built-in policy decides whether
// that location may be written. An absolute path under an allowed root is the
// case agents kept failing on, and it has to survive both stages; a path under
// a deny root has to be refused by the second one, since the first no longer
// looks at the shape at all.
func TestDownloadOutputReachesThePathPolicy(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}

	// The working directory is an allow root on every platform, which the
	// platform temp directory is not: os.TempDir() answers $TMPDIR on macOS,
	// and that is outside the allowlist.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	allowed := filepath.Join(cwd, "im-dl-probe.bin")
	denied := filepath.Join(home, ".ssh", "im-dl-probe.bin")

	for _, tc := range []struct {
		name   string
		output string
	}{
		{"absolute path under an allowed root", allowed},
		{"absolute path in a credential directory", denied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Stage one keeps the caller's path verbatim either way.
			got, err := normalizeDownloadOutputPath("file_123", tc.output)
			if err != nil {
				t.Fatalf("normalizeDownloadOutputPath(%q) error = %v, want the path passed through", tc.output, err)
			}
			if got != filepath.Clean(tc.output) {
				t.Fatalf("normalizeDownloadOutputPath(%q) = %q, want it unchanged", tc.output, got)
			}
		})
	}

	// Stage two is where the verdict is made. Both paths are absolute, so a
	// difference here can only come from the policy.
	if _, err := validate.SafeOutputPath(allowed); err != nil {
		t.Errorf("SafeOutputPath(%q) error = %v, want an allowed root to be writable", allowed, err)
	}
	_, denyErr := validate.SafeOutputPath(denied)
	if denyErr == nil {
		t.Fatalf("SafeOutputPath(%q) = nil error, want the denylist to refuse it", denied)
	}
	if !strings.Contains(denyErr.Error(), "denylist") {
		t.Errorf("refusal should cite the denylist, got: %v", denyErr)
	}
}

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
