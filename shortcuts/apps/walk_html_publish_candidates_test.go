// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
)

// permissiveFIO is a test-only fileio that delegates to os without
// SafeInputPath validation. Unit tests use it so we can drive the walker
// and tarball algorithms with absolute t.TempDir paths; production code
// goes through LocalFileIO which is cwd-bounded.
type permissiveFIO struct{}

func (permissiveFIO) Open(name string) (fileio.File, error)     { return os.Open(name) }
func (permissiveFIO) Stat(name string) (fileio.FileInfo, error) { return os.Stat(name) }
func (permissiveFIO) ResolvePath(p string) (string, error)      { return p, nil }
func (permissiveFIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	panic("Save not used in apps unit tests")
}

func newTestFIO() fileio.FileIO { return permissiveFIO{} }

func TestWalkHTMLPublishCandidates_SingleFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "index.html")
	if err := os.WriteFile(file, []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := walkHTMLPublishCandidates(newTestFIO(), file)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(got) != 1 || got[0].RelPath != "index.html" || got[0].Size != 13 {
		t.Fatalf("got=%+v", got)
	}
}

func TestWalkHTMLPublishCandidates_Directory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"index.html":      "<html></html>",
		"css/main.css":    "body{}",
		"assets/logo.svg": "<svg/>",
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	got, err := walkHTMLPublishCandidates(newTestFIO(), dir)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d candidates, want 3", len(got))
	}
	rels := make([]string, 3)
	for i, c := range got {
		rels[i] = c.RelPath
	}
	sort.Strings(rels)
	want := []string{"assets/logo.svg", "css/main.css", "index.html"}
	for i, w := range want {
		if rels[i] != w {
			t.Fatalf("rel[%d]=%q want %q", i, rels[i], w)
		}
	}
}

func TestWalkHTMLPublishCandidates_NotFound(t *testing.T) {
	if _, err := walkHTMLPublishCandidates(newTestFIO(), "/nonexistent/xyz"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWalkHTMLPublishCandidates_SymlinkSkipped(t *testing.T) {
	// Walker 只接受 regular file —— symlink 跳过（避免 loop + out-of-root 引用，
	// 且 fio.Open 对 symlink 行为不一致）。real.html 仍然被收，link.html 不在结果里。
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "real.html"), filepath.Join(dir, "link.html")); err != nil {
		t.Skipf("symlink not supported on this filesystem: %v", err)
	}
	got, err := walkHTMLPublishCandidates(newTestFIO(), dir)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	rels := make(map[string]bool)
	for _, c := range got {
		rels[c.RelPath] = true
	}
	if !rels["real.html"] {
		t.Fatalf("expected real.html (regular file) in candidates, got %+v", got)
	}
	if rels["link.html"] {
		t.Fatalf("symlink link.html should NOT appear in candidates, got %+v", got)
	}
}
