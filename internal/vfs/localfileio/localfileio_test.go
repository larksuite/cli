// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/vfs"
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
	_, err := fio.Open("../../../../../../../../../../../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestLocalFileIO_Open_RejectsDenylistedAbsolutePath(t *testing.T) {
	fio := &LocalFileIO{}
	_, err := fio.Open(denylistedAbsolutePath(t))
	if err == nil {
		t.Error("expected error for denylisted absolute path")
	}
	if err != nil && !strings.Contains(err.Error(), "denylist") {
		t.Errorf("error should mention the denylist, got: %v", err)
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
	_, err := fio.Stat("../../../../../../../../../../../../etc/passwd")
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
	_, err := fio.Save("../../../../../../../../../../../../evil.txt", fileio.SaveOptions{}, strings.NewReader("bad"))
	if err == nil {
		t.Error("expected error for path traversal in Save")
	}
}

func TestLocalFileIO_Save_RejectsPathOutsideAllowlist(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	// The home directory itself is not an allow root (only ~/files is), so
	// this absolute target must be refused before anything touches the disk.
	target := filepath.Join(home, "save-outside-allowlist-test", "evil.txt")

	fio := &LocalFileIO{}
	if _, err := fio.Save(target, fileio.SaveOptions{}, strings.NewReader("bad")); err == nil {
		t.Error("expected error for absolute path outside the allowlist in Save")
	}
	if _, err := os.Stat(filepath.Dir(target)); !os.IsNotExist(err) {
		t.Errorf("Save must not create directories for rejected paths, stat err = %v", err)
	}
}

func TestLocalFileIO_RemoveWorkspaceEntry_IsValidatedAndNonRecursive(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	workspace := "draft_workspace"
	decisionPath := filepath.Join(workspace, ".presentation-decision.json")
	keepPath := filepath.Join(workspace, "keep.txt")
	if _, err := fio.Save(decisionPath, fileio.SaveOptions{}, strings.NewReader("{}")); err != nil {
		t.Fatalf("Save decision: %v", err)
	}
	if _, err := fio.Save(keepPath, fileio.SaveOptions{}, strings.NewReader("keep")); err != nil {
		t.Fatalf("Save retained entry: %v", err)
	}

	if err := fio.RemoveWorkspaceEntry(decisionPath); err != nil {
		t.Fatalf("RemoveWorkspaceEntry file: %v", err)
	}
	if _, err := os.Stat(decisionPath); !os.IsNotExist(err) {
		t.Fatalf("removed decision still exists or stat failed unexpectedly: %v", err)
	}
	if err := fio.RemoveWorkspaceEntry(workspace); err == nil {
		t.Fatal("RemoveWorkspaceEntry recursively removed a non-empty directory")
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("non-recursive removal deleted retained entry: %v", err)
	}
	if err := fio.RemoveWorkspaceEntry(keepPath); err != nil {
		t.Fatalf("RemoveWorkspaceEntry retained file: %v", err)
	}
	if err := fio.RemoveWorkspaceEntry(workspace); err != nil {
		t.Fatalf("RemoveWorkspaceEntry empty directory: %v", err)
	}
	if err := fio.RemoveWorkspaceEntry("../../../../../../../../../../../../outside"); !errors.Is(err, fileio.ErrPathValidation) {
		t.Fatalf("traversal error = %v, want fileio.ErrPathValidation", err)
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
	_, err := fio.ResolvePath("../../../../../../../../../../../../etc/passwd")
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

func TestAppendToCreatesAndAppends(t *testing.T) {
	io := &LocalFileIO{}
	dir := t.TempDir()
	testChdir(t, dir)
	p := "partial.bin"

	res, err := io.AppendTo(p, fileio.SaveOptions{}, strings.NewReader("hello "))
	if err != nil {
		t.Fatalf("AppendTo() error = %v", err)
	}
	if res.Size() != 6 {
		t.Fatalf("first append size = %d, want 6", res.Size())
	}
	res, err = io.AppendTo(p, fileio.SaveOptions{}, strings.NewReader("world"))
	if err != nil {
		t.Fatalf("AppendTo() error = %v", err)
	}
	if res.Size() != 5 {
		t.Fatalf("second append size = %d, want 5", res.Size())
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("content = %q, want %q", got, "hello world")
	}
}

func TestAppendToRejectsUnsafePath(t *testing.T) {
	io := &LocalFileIO{}
	_, err := io.AppendTo("/absolute/unsafe", fileio.SaveOptions{}, strings.NewReader("x"))
	if err == nil {
		t.Fatal("expected unsafe path error")
	}
	var pathErr *fileio.PathValidationError
	if !errors.As(err, &pathErr) {
		t.Fatalf("error = %T %v, want *fileio.PathValidationError", err, err)
	}
	if pathErr.Err == nil || !errors.Is(pathErr.Err, fs.ErrPermission) && pathErr.Err.Error() == "" {
		t.Fatalf("path validation error lacks a wrapped cause: %+v", pathErr)
	}
}

// TestAppendToKeepsWrittenBytesOnFailure verifies that a reader which fails
// mid-stream leaves the bytes it already wrote on disk, so a resumable
// download can continue from the same offset on the next attempt.
func TestAppendToKeepsWrittenBytesOnFailure(t *testing.T) {
	io := &LocalFileIO{}
	dir := t.TempDir()
	testChdir(t, dir)
	p := "partial.bin"
	boom := errors.New("boom")
	body := &prefixErrReader{prefix: strings.NewReader("persisted-"), err: boom}
	_, err := io.AppendTo(p, fileio.SaveOptions{}, body)
	if err == nil {
		t.Fatal("expected append error")
	}
	var writeErr *fileio.WriteError
	if !errors.As(err, &writeErr) {
		t.Fatalf("error = %T %v, want *fileio.WriteError", err, err)
	}
	if !errors.Is(writeErr.Err, boom) {
		t.Fatalf("WriteError.Err = %v, want the underlying reader error to be preserved", writeErr.Err)
	}
	got, rerr := vfs.ReadFile(p)
	if rerr != nil {
		t.Fatalf("ReadFile() error = %v", rerr)
	}
	if string(got) != "persisted-" {
		t.Fatalf("content = %q, want the prefix written before the failure", got)
	}
}

func TestLocalFileIO_ResumableArtifactsAndCommit(t *testing.T) {
	dir := t.TempDir()
	testChdir(t, dir)

	fio := &LocalFileIO{}
	if err := fio.WriteResumeArtifact("out.bin.partial.meta", []byte(`{"size":5,"etag":"\"v1\""}`)); err != nil {
		t.Fatalf("WriteResumeArtifact() error = %v", err)
	}
	data, err := fio.ReadResumeArtifact("out.bin.partial.meta")
	if err != nil {
		t.Fatalf("ReadResumeArtifact() error = %v", err)
	}
	if string(data) != `{"size":5,"etag":"\"v1\""}` {
		t.Fatalf("checkpoint = %q", data)
	}
	if _, err := fio.AppendTo("out.bin.partial", fileio.SaveOptions{}, strings.NewReader("new")); err != nil {
		t.Fatalf("AppendTo() error = %v", err)
	}
	if err := fio.CommitResumeArtifact("out.bin.partial", "out.bin", false); err != nil {
		t.Fatalf("CommitResumeArtifact() error = %v", err)
	}
	got, err := os.ReadFile("out.bin")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("committed content = %q, want new", got)
	}
	if _, err := os.Stat("out.bin.partial"); !os.IsNotExist(err) {
		t.Fatalf("partial still exists after commit: %v", err)
	}
	if err := fio.RemoveResumeArtifact("out.bin.partial.meta"); err != nil {
		t.Fatalf("RemoveResumeArtifact() error = %v", err)
	}

	if _, err := fio.AppendTo("out.bin.partial", fileio.SaveOptions{}, strings.NewReader("replacement")); err != nil {
		t.Fatalf("AppendTo(replacement) error = %v", err)
	}
	if err := fio.CommitResumeArtifact("out.bin.partial", "out.bin", false); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("no-overwrite commit error = %v, want fs.ErrExist", err)
	}
	if err := fio.CommitResumeArtifact("out.bin.partial", "out.bin", true); err != nil {
		t.Fatalf("overwrite commit error = %v", err)
	}
	got, err = os.ReadFile("out.bin")
	if err != nil {
		t.Fatalf("ReadFile(overwrite) error = %v", err)
	}
	if string(got) != "replacement" {
		t.Fatalf("overwritten content = %q, want replacement", got)
	}

	if err := os.WriteFile("out.bin", []byte("keep"), 0600); err != nil {
		t.Fatalf("WriteFile(keep) error = %v", err)
	}
	if err := fio.CommitResumeArtifact("missing.bin.partial", "out.bin", true); err == nil {
		t.Fatal("CommitResumeArtifact() unexpectedly succeeded with a missing partial")
	}
	got, err = os.ReadFile("out.bin")
	if err != nil {
		t.Fatalf("ReadFile(after failed overwrite) error = %v", err)
	}
	if string(got) != "keep" {
		t.Fatalf("target after failed overwrite = %q, want keep", got)
	}
}

// prefixErrReader yields prefix bytes first, then reports err.
type prefixErrReader struct {
	prefix io.Reader
	err    error
}

// Read yields the prefix bytes first and then reports the configured error.
func (r *prefixErrReader) Read(b []byte) (int, error) {
	if r.prefix != nil {
		n, err := r.prefix.Read(b)
		if err == io.EOF {
			r.prefix = nil
			return n, nil
		}
		return n, err
	}
	return 0, r.err
}
