// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/vfs"
)

func TestOpenLocalFile_AcceptsAbsoluteAndParentRelativePaths(t *testing.T) {
	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "input.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	TestChdir(t, workDir)

	for _, input := range []string{path, filepath.Join("..", "input.txt")} {
		f, err := OpenLocalFile(input)
		if err != nil {
			t.Fatalf("OpenLocalFile(%q) error = %v", input, err)
		}
		got, readErr := io.ReadAll(f)
		closeErr := f.Close()
		if readErr != nil || closeErr != nil || string(got) != "content" {
			t.Fatalf("OpenLocalFile(%q) content=%q read=%v close=%v", input, got, readErr, closeErr)
		}
	}
}

func TestOpenLocalFile_RejectsInvalidInput(t *testing.T) {
	if _, err := OpenLocalFile("input\n.txt"); !errors.Is(err, fileio.ErrPathValidation) {
		t.Fatalf("OpenLocalFile() error = %v, want ErrPathValidation", err)
	}
}

func TestStatLocalFile_ReturnsMetadata(t *testing.T) {
	info, err := StatLocalFile(t.TempDir())
	if err != nil {
		t.Fatalf("StatLocalFile() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("StatLocalFile() mode = %v, want directory", info.Mode())
	}
}

func TestOpenLocalFile_DoesNotStatBeforeOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	previous := vfs.DefaultFS
	counting := &countingLocalFileFS{FS: previous}
	vfs.DefaultFS = counting
	t.Cleanup(func() { vfs.DefaultFS = previous })

	f, err := OpenLocalFile(path)
	if err != nil {
		t.Fatalf("OpenLocalFile() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if counting.openCalls != 1 || counting.statCalls != 0 {
		t.Fatalf("OpenLocalFile() calls: Open=%d Stat=%d, want Open=1 Stat=0", counting.openCalls, counting.statCalls)
	}
}

type countingLocalFileFS struct {
	vfs.FS
	openCalls int
	statCalls int
}

func (f *countingLocalFileFS) Open(name string) (*os.File, error) {
	f.openCalls++
	return f.FS.Open(name)
}

func (f *countingLocalFileFS) Stat(name string) (fs.FileInfo, error) {
	f.statCalls++
	return f.FS.Stat(name)
}
