// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/tar"
	"bytes"
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

	fio := newTestFIO()
	candidates, err := walkHTMLPublishCandidates(fio, dir)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	tarball, err := buildHTMLPublishTarball(fio, candidates)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(tarball.SHA256) != 64 {
		t.Fatalf("SHA256 wrong len: %d", len(tarball.SHA256))
	}
	if tarball.Size <= 0 || int64(len(tarball.Body)) != tarball.Size {
		t.Fatalf("size=%d body=%d", tarball.Size, len(tarball.Body))
	}

	gz, err := gzip.NewReader(bytes.NewReader(tarball.Body))
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
	if _, err := buildHTMLPublishTarball(newTestFIO(), nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteHTMLPublishTarEntry_OpenFailure(t *testing.T) {
	// candidate 指向不存在文件 → fio.Open 失败 → 错误返回
	tw := tar.NewWriter(io.Discard)
	defer tw.Close()
	err := writeHTMLPublishTarEntry(newTestFIO(), tw, htmlPublishCandidate{
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

	err := writeHTMLPublishTarEntry(newTestFIO(), tw, htmlPublishCandidate{
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
	// 文件在 Open 时存在，但读取时失败（chmod 0 模拟权限错）
	dir := t.TempDir()
	file := filepath.Join(dir, "x.html")
	if err := os.WriteFile(file, []byte("content"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_ = os.Chmod(file, 0o000)
	defer os.Chmod(file, 0o644)

	tw := tar.NewWriter(io.Discard)
	defer tw.Close()

	err := writeHTMLPublishTarEntry(newTestFIO(), tw, htmlPublishCandidate{
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

func TestBuildHTMLPublishTarball_EntryWriteFailureReturnsError(t *testing.T) {
	// candidate 指向不存在文件 → writeHTMLPublishTarEntry 失败
	// → buildHTMLPublishTarball 返回 nil tarball + error。
	// 内存打包不再创建临时文件，无清理路径需要验证。
	candidates := []htmlPublishCandidate{
		{RelPath: "x.html", AbsPath: "/nonexistent-path-for-test/x.html", Size: 0},
	}

	tarball, err := buildHTMLPublishTarball(newTestFIO(), candidates)
	if err == nil {
		t.Fatalf("expected error, got tarball=%+v", tarball)
	}
	if tarball != nil {
		t.Fatalf("expected nil tarball on error, got %+v", tarball)
	}
}
