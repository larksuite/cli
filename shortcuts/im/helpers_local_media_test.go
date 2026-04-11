// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

type countingOpenFS struct {
	vfs.OsFs
	cwd       string
	openCalls int
}

func (fs *countingOpenFS) Getwd() (string, error) {
	return fs.cwd, nil
}

func (fs *countingOpenFS) Open(name string) (*os.File, error) {
	fs.openCalls++
	return nil, os.ErrPermission
}

func TestResolveLocalMedia_ValidatesPathBeforeParsingDuration(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "work")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	outside := filepath.Join(root, "outside.mp4")
	if err := os.WriteFile(outside, []byte("not-a-real-mp4"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	mockFS := &countingOpenFS{cwd: cwd}
	oldFS := vfs.DefaultFS
	vfs.DefaultFS = mockFS
	t.Cleanup(func() { vfs.DefaultFS = oldFS })

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	runtime := &common.RuntimeContext{Factory: f}
	spec := mediaSpec{
		value:        "../outside.mp4",
		mediaType:    "video",
		kind:         mediaKindFile,
		withDuration: true,
	}

	_, err := resolveLocalMedia(context.Background(), runtime, spec)
	if err == nil {
		t.Fatal("expected path validation error")
	}
	if !strings.Contains(err.Error(), "resolves outside the current working directory") {
		t.Fatalf("error = %v, want path validation error", err)
	}
	if mockFS.openCalls != 0 {
		t.Fatalf("Open() called %d times, want 0 before validation", mockFS.openCalls)
	}
}
