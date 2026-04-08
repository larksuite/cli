// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
)

// testChdir temporarily changes the working directory for a test.
// Not compatible with t.Parallel().
func testChdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

// ── Provider ──

func TestProvider_Name(t *testing.T) {
	p := &Provider{}
	if got := p.Name(); got != "local" {
		t.Errorf("Provider.Name() = %q, want %q", got, "local")
	}
}

func TestProvider_ResolveFileIO(t *testing.T) {
	p := &Provider{}
	fio := p.ResolveFileIO(nil)
	if fio == nil {
		t.Fatal("Provider.ResolveFileIO returned nil")
	}
	if _, ok := fio.(*LocalFileIO); !ok {
		t.Errorf("expected *LocalFileIO, got %T", fio)
	}
}

// ── Open ──

func TestLocalFileIO_Open_ValidFile(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	content := []byte("hello world")
	os.WriteFile("test.txt", content, 0644)

	fio := &LocalFileIO{}
	f, err := fio.Open("test.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestLocalFileIO_Open_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	_, err := fio.Open("../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestLocalFileIO_Open_RejectsAbsolutePath(t *testing.T) {
	fio := &LocalFileIO{}
	_, err := fio.Open("/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute path")
	}
	if err != nil && !strings.Contains(err.Error(), "relative path") {
		t.Errorf("error should mention relative path, got: %v", err)
	}
}

func TestLocalFileIO_Open_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	_, err := fio.Open("nonexistent.txt")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// ── Stat ──

func TestLocalFileIO_Stat_ValidFile(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	os.WriteFile("stat.txt", []byte("12345"), 0644)

	fio := &LocalFileIO{}
	info, err := fio.Stat("stat.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != 5 {
		t.Errorf("Size() = %d, want 5", info.Size())
	}
	if info.IsDir() {
		t.Error("expected IsDir() = false")
	}
}

func TestLocalFileIO_Stat_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	_, err := fio.Stat("../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
	if err != nil && os.IsNotExist(err) {
		t.Error("traversal should not be os.IsNotExist, should be a validation error")
	}
}

func TestLocalFileIO_Stat_NonexistentReturnsIsNotExist(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	_, err := fio.Stat("nope.txt")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist, got: %v", err)
	}
}

// ── Save ──

func TestLocalFileIO_Save_WritesContent(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	body := strings.NewReader("saved content")
	result, err := fio.Save("output.bin", fileio.SaveOptions{}, body)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if result.Size() != int64(len("saved content")) {
		t.Errorf("Size() = %d, want %d", result.Size(), len("saved content"))
	}

	got, _ := os.ReadFile(filepath.Join(dir, "output.bin"))
	if string(got) != "saved content" {
		t.Errorf("file content = %q, want %q", got, "saved content")
	}
}

func TestLocalFileIO_Save_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	body := strings.NewReader("nested")
	_, err := fio.Save(filepath.Join("a", "b", "c.txt"), fileio.SaveOptions{}, body)
	if err != nil {
		t.Fatalf("Save with nested dir failed: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "a", "b", "c.txt"))
	if string(got) != "nested" {
		t.Errorf("file content = %q, want %q", got, "nested")
	}
}

func TestLocalFileIO_Save_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	_, err := fio.Save("../../evil.txt", fileio.SaveOptions{}, strings.NewReader("bad"))
	if err == nil {
		t.Error("expected error for path traversal in Save")
	}
}

func TestLocalFileIO_Save_RejectsAbsolutePath(t *testing.T) {
	fio := &LocalFileIO{}
	_, err := fio.Save("/tmp/evil.txt", fileio.SaveOptions{}, strings.NewReader("bad"))
	if err == nil {
		t.Error("expected error for absolute path in Save")
	}
}

// ── ResolvePath ──

func TestLocalFileIO_ResolvePath_ReturnsAbsolute(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	resolved, err := fio.ResolvePath("file.txt")
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}
	if !filepath.IsAbs(resolved) {
		t.Errorf("expected absolute path, got %q", resolved)
	}
	if filepath.Base(resolved) != "file.txt" {
		t.Errorf("expected base name file.txt, got %q", filepath.Base(resolved))
	}
}

func TestLocalFileIO_ResolvePath_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	_, err := fio.ResolvePath("../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal in ResolvePath")
	}
}

func TestLocalFileIO_ResolvePath_RejectsAbsolute(t *testing.T) {
	fio := &LocalFileIO{}
	_, err := fio.ResolvePath("/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute path in ResolvePath")
	}
}

// ── Error message consistency ──

func TestLocalFileIO_ErrorMessages_ContainCorrectFlagName(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}

	// Open/Stat use SafeInputPath → errors should mention "--file"
	_, err := fio.Open("/absolute/path")
	if err == nil || !strings.Contains(err.Error(), "--file") {
		t.Errorf("Open absolute path error should mention --file, got: %v", err)
	}

	_, err = fio.Stat("/absolute/path")
	if err == nil || !strings.Contains(err.Error(), "--file") {
		t.Errorf("Stat absolute path error should mention --file, got: %v", err)
	}

	// Save/ResolvePath use SafeOutputPath → errors should mention "--output"
	_, err = fio.Save("/absolute/path", fileio.SaveOptions{}, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "--output") {
		t.Errorf("Save absolute path error should mention --output, got: %v", err)
	}

	_, err = fio.ResolvePath("/absolute/path")
	if err == nil || !strings.Contains(err.Error(), "--output") {
		t.Errorf("ResolvePath absolute path error should mention --output, got: %v", err)
	}
}

// ── Control character / Unicode rejection ──

func TestLocalFileIO_RejectsControlCharsInPath(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	paths := []string{
		"file\x00name.txt",   // null byte
		"file\x1fname.txt",   // control char
		"file\u200Bname.txt", // zero-width space
		"file\u202Ename.txt", // bidi override
	}

	for _, p := range paths {
		if _, err := fio.Open(p); err == nil {
			t.Errorf("Open(%q) should reject control/dangerous chars", p)
		}
		if _, err := fio.Save(p, fileio.SaveOptions{}, strings.NewReader("")); err == nil {
			t.Errorf("Save(%q) should reject control/dangerous chars", p)
		}
	}
}
