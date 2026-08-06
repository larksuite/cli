// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentartifact

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestWriteTempMarkdownWritesExactPrivateFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	if runtime.GOOS == "windows" {
		t.Setenv("TEMP", tempDir)
		t.Setenv("TMP", tempDir)
	}

	content := []byte("# Heading\n\nbody without a trailing newline")
	path, size, err := WriteTempMarkdown(content)
	if err != nil {
		t.Fatalf("WriteTempMarkdown() error = %v", err)
	}
	t.Cleanup(func() { _ = vfs.Remove(path) })

	if !filepath.IsAbs(path) {
		t.Errorf("path = %q, want an absolute path", path)
	}
	if dir := filepath.Clean(filepath.Dir(path)); dir != filepath.Clean(tempDir) {
		t.Errorf("directory = %q, want %q", dir, tempDir)
	}
	if name := filepath.Base(path); !strings.HasPrefix(name, "lark-cli-fetch-") || filepath.Ext(name) != ".md" {
		t.Errorf("filename = %q, want lark-cli-fetch-*.md", name)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}

	got, err := vfs.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want exact bytes %q", got, content)
	}

	if runtime.GOOS != "windows" {
		info, err := vfs.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v", path, err)
		}
		if permission := info.Mode().Perm(); permission != 0o600 {
			t.Errorf("permission = %04o, want 0600", permission)
		}
	}
}

func TestWriteTempMarkdownRemovesPartialFileAfterFailure(t *testing.T) {
	testErr := errors.New("injected failure")
	tests := []struct {
		name       string
		configure  func(*fakeTempFile)
		wantCloses int
	}{
		{
			name: "chmod",
			configure: func(file *fakeTempFile) {
				file.chmodErr = testErr
			},
			wantCloses: 1,
		},
		{
			name: "write",
			configure: func(file *fakeTempFile) {
				file.writeErr = testErr
			},
			wantCloses: 1,
		},
		{
			name: "short write",
			configure: func(file *fakeTempFile) {
				file.shortWrite = true
			},
			wantCloses: 1,
		},
		{
			name: "sync",
			configure: func(file *fakeTempFile) {
				file.syncErr = testErr
			},
			wantCloses: 1,
		},
		{
			name: "close",
			configure: func(file *fakeTempFile) {
				file.closeErr = testErr
			},
			wantCloses: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &fakeTempFile{name: "partial.md"}
			tt.configure(file)
			var removed []string

			_, _, err := writeTempMarkdown(
				[]byte("content"),
				func(_, _ string) (tempFile, error) { return file, nil },
				func() (string, error) { return "/tmp", nil },
				func(path string) error {
					removed = append(removed, path)
					return nil
				},
			)
			if err == nil {
				t.Fatal("writeTempMarkdown() error = nil, want failure")
			}
			if len(removed) != 1 || removed[0] != file.name {
				t.Errorf("removed = %v, want [%q]", removed, file.name)
			}
			if file.closeCalls != tt.wantCloses {
				t.Errorf("close calls = %d, want %d", file.closeCalls, tt.wantCloses)
			}
		})
	}
}

func TestWriteTempMarkdownCreateFailureHasNoCleanup(t *testing.T) {
	testErr := errors.New("create failed")
	removeCalls := 0

	_, _, err := writeTempMarkdown(
		[]byte("content"),
		func(_, _ string) (tempFile, error) { return nil, testErr },
		func() (string, error) { return "/tmp", nil },
		func(string) error {
			removeCalls++
			return nil
		},
	)
	if !errors.Is(err, testErr) {
		t.Fatalf("error = %v, want %v", err, testErr)
	}
	if removeCalls != 0 {
		t.Errorf("remove calls = %d, want 0", removeCalls)
	}
}

func TestWriteTempMarkdownResolvesRelativePath(t *testing.T) {
	file := &fakeTempFile{name: filepath.Join("tmp", "artifact.md")}
	path, size, err := writeTempMarkdown(
		[]byte("abc"),
		func(_, _ string) (tempFile, error) { return file, nil },
		func() (string, error) { return filepath.FromSlash("/workspace"), nil },
		func(string) error { return nil },
	)
	if err != nil {
		t.Fatalf("writeTempMarkdown() error = %v", err)
	}
	if want := filepath.Join(filepath.FromSlash("/workspace"), file.name); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if size != 3 {
		t.Errorf("size = %d, want 3", size)
	}
}

type fakeTempFile struct {
	name       string
	chmodErr   error
	writeErr   error
	shortWrite bool
	syncErr    error
	closeErr   error
	closeCalls int
}

func (f *fakeTempFile) Name() string { return f.name }

func (f *fakeTempFile) Chmod(fs.FileMode) error { return f.chmodErr }

func (f *fakeTempFile) Write(content []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.shortWrite {
		return len(content) - 1, nil
	}
	return len(content), nil
}

func (f *fakeTempFile) Sync() error { return f.syncErr }

func (f *fakeTempFile) Close() error {
	f.closeCalls++
	return f.closeErr
}

var _ tempFile = (*os.File)(nil)
var _ tempFile = (*fakeTempFile)(nil)
