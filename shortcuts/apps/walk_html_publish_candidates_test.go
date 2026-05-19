// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestWalkHTMLPublishCandidates_SingleFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "index.html")
	if err := os.WriteFile(file, []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := walkHTMLPublishCandidates(file)
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

	got, err := walkHTMLPublishCandidates(dir)
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
	if _, err := walkHTMLPublishCandidates("/nonexistent/xyz"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWalkHTMLPublishCandidates_Symlink(t *testing.T) {
	// 验证 filepath.WalkDir 默认行为：不跟随符号链接。
	// 这是设计决策——避免 symlink loop / out-of-root 引用。
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "real.html"), filepath.Join(dir, "link.html")); err != nil {
		t.Skipf("symlink not supported on this filesystem: %v", err)
	}
	got, err := walkHTMLPublishCandidates(dir)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	// 我们仍然会列出 link.html（它是文件入口），但 WalkDir 不会沿着链接进入目标目录递归。
	// 这里关键是：不能 panic、不能死循环、不能跨链接复制内容。Size 0 表示 symlink 自身长度（Info().Size 对 symlink 报符号长度），
	// 这里不严格断言 size — 只断言操作不报错且包含 link.html 名字。
	found := false
	for _, c := range got {
		if c.RelPath == "link.html" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected link.html in candidates, got %+v", got)
	}
}
