// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestIsUnsafeRel(t *testing.T) {
	unsafe := []string{"/abs", "..", "../x", "a/../../b", "a/..", "a\x00b"}
	for _, in := range unsafe {
		if !isUnsafeRel(in) {
			t.Errorf("isUnsafeRel(%q) = false, want true", in)
		}
	}
	safe := []string{"index.html", "a/b.css", "archive.tar..bak"}
	for _, in := range safe {
		if isUnsafeRel(in) {
			t.Errorf("isUnsafeRel(%q) = true, want false", in)
		}
	}
}

func TestCollectDirSkipsGitAndNonRegular(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "index.html"), "hi")
	mustWrite(t, filepath.Join(root, "assets", "x.css"), "c")
	mustWrite(t, filepath.Join(root, ".git", "config"), "g")
	if err := os.Symlink(filepath.Join(root, "index.html"), filepath.Join(root, "link.html")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	cands, rootNames, _, err := collectDirAt(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := map[string]bool{}
	for _, c := range cands {
		got[c.RelPath] = true
	}
	if !got["index.html"] || !got["assets/x.css"] {
		t.Errorf("missing expected files: %v", got)
	}
	if got[".git/config"] {
		t.Error(".git subtree must be skipped")
	}
	if got["link.html"] {
		t.Error("symlinks must not be followed")
	}
	if len(rootNames) == 0 {
		t.Error("rootNames must list the directory's top-level file names")
	}
}
