// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildHTMLPublishTarball_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	candidates, err := walkHTMLPublishCandidates(dir)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	tarball, err := buildHTMLPublishTarball(candidates)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(tarball.Path) })

	if len(tarball.SHA256) != 64 {
		t.Fatalf("SHA256 wrong len: %d", len(tarball.SHA256))
	}
	if tarball.Size <= 0 {
		t.Fatalf("size=%d", tarball.Size)
	}

	f, err := os.Open(tarball.Path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tar.Next: %v", err)
	}
	if hdr.Name != "index.html" {
		t.Fatalf("entry name = %q, want index.html", hdr.Name)
	}
	body, err := io.ReadAll(tr)
	if err != nil || string(body) != "<html></html>" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestBuildHTMLPublishTarball_EmptyCandidates(t *testing.T) {
	if _, err := buildHTMLPublishTarball(nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteHTMLPublishTarEntry_OpenFailure(t *testing.T) {
	// candidate 指向不存在文件 → os.Open 失败 → 错误返回
	tw := tar.NewWriter(io.Discard)
	defer tw.Close()
	err := writeHTMLPublishTarEntry(tw, htmlPublishCandidate{
		RelPath: "x.html",
		AbsPath: "/nonexistent-path-for-test/x.html",
		Size:    0,
	})
	if err == nil {
		t.Fatalf("expected error for nonexistent abs path")
	}
	if !strings.Contains(err.Error(), "open") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestWriteHTMLPublishTarEntry_WriteHeaderFailure(t *testing.T) {
	// 在已 close 的 tar.Writer 上写 header → WriteHeader 失败
	dir := t.TempDir()
	file := filepath.Join(dir, "x.html")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tw := tar.NewWriter(io.Discard)
	_ = tw.Close() // 先 close，下次 WriteHeader 必失败

	err := writeHTMLPublishTarEntry(tw, htmlPublishCandidate{
		RelPath: "x.html",
		AbsPath: file,
		Size:    1,
	})
	if err == nil {
		t.Fatalf("expected error when writing to closed tar.Writer")
	}
	if !strings.Contains(err.Error(), "write header") {
		t.Fatalf("expected 'write header' error, got %v", err)
	}
}

func TestWriteHTMLPublishTarEntry_CopyFailure(t *testing.T) {
	// 文件在 Open 时存在，但写到 tar 时中断（类似 io.Reader 在读取时失败）
	// 这里我们用一个特殊的错误 Reader 来模拟
	dir := t.TempDir()
	file := filepath.Join(dir, "x.html")
	if err := os.WriteFile(file, []byte("content"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// 创建一个会在读取时失败的 Reader（改变权限使其无法读）
	_ = os.Chmod(file, 0o000)
	defer os.Chmod(file, 0o644) // 恢复权限便于清理

	tw := tar.NewWriter(io.Discard)
	defer tw.Close()

	err := writeHTMLPublishTarEntry(tw, htmlPublishCandidate{
		RelPath: "x.html",
		AbsPath: file,
		Size:    7,
	})
	if err == nil {
		t.Fatalf("expected error for permission denied file")
	}
	if !strings.Contains(err.Error(), "open") {
		t.Fatalf("expected open/permission error, got %v", err)
	}
}

func TestBuildHTMLPublishTarball_EntryWriteFailureCleansUp(t *testing.T) {
	// candidate 指向不存在文件 → writeHTMLPublishTarEntry 失败
	// → buildHTMLPublishTarball 走错误清理路径（tw.Close / gz.Close / tmp.Close / defer os.Remove）
	candidates := []htmlPublishCandidate{
		{RelPath: "x.html", AbsPath: "/nonexistent-path-for-test/x.html", Size: 0},
	}

	pattern := filepath.Join(os.TempDir(), "apps-html-publish-*.tar.gz")
	before, _ := filepath.Glob(pattern)

	tarball, err := buildHTMLPublishTarball(candidates)
	if err == nil {
		t.Fatalf("expected error, got tarball=%+v", tarball)
	}
	if tarball != nil {
		t.Fatalf("expected nil tarball on error, got %+v", tarball)
	}

	// 验证错误回退路径仍清理了临时文件（没有 leak）
	after, _ := filepath.Glob(pattern)
	if len(after) > len(before) {
		t.Fatalf("error path leaked tarball: before=%d after=%d", len(before), len(after))
	}
}
