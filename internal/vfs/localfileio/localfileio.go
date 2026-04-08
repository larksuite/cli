// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"context"
	"io"
	"path/filepath"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/vfs"
)

// Provider is the default fileio.Provider backed by the local filesystem.
type Provider struct{}

func (p *Provider) Name() string { return "local" }

func (p *Provider) ResolveFileIO(_ context.Context) fileio.FileIO {
	return &LocalFileIO{}
}

func init() {
	fileio.Register(&Provider{})
}

// LocalFileIO implements fileio.FileIO using the local filesystem.
// Path validation (SafeInputPath/SafeOutputPath), directory creation,
// and atomic writes are handled internally.
type LocalFileIO struct{}

// Open opens a local file for reading after validating the path.
func (l *LocalFileIO) Open(name string) (fileio.File, error) {
	safePath, err := SafeInputPath(name)
	if err != nil {
		return nil, err
	}
	return vfs.Open(safePath)
}

// Stat returns file metadata after validating the path.
func (l *LocalFileIO) Stat(name string) (fileio.FileInfo, error) {
	safePath, err := SafeInputPath(name)
	if err != nil {
		return nil, err
	}
	return vfs.Stat(safePath)
}

// saveResult implements fileio.SaveResult.
type saveResult struct{ size int64 }

func (r *saveResult) Size() int64 { return r.size }

// ResolvePath returns the validated absolute path for the given output path.
func (l *LocalFileIO) ResolvePath(path string) (string, error) {
	return SafeOutputPath(path)
}

// Save writes body to path atomically after validating the output path.
// Parent directories are created as needed. The body is streamed directly
// to a temp file and renamed, avoiding full in-memory buffering.
func (l *LocalFileIO) Save(path string, _ fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	safePath, err := SafeOutputPath(path)
	if err != nil {
		return nil, err
	}
	if err := vfs.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
		return nil, err
	}
	n, err := AtomicWriteFromReader(safePath, body, 0600)
	if err != nil {
		return nil, err
	}
	return &saveResult{size: n}, nil
}
