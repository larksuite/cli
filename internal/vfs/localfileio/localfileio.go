// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"context"
	"errors"
	"io"
	"io/fs"
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

var _ fileio.WorkspaceFileIO = (*LocalFileIO)(nil)

// Open opens a local file for reading after validating the path. The open
// itself is hardened (openValidated): the fd is verified against the
// validation-stage stat and must be a regular file.
func (l *LocalFileIO) Open(name string) (fileio.File, error) {
	safePath, err := SafeInputPath(name)
	if err != nil {
		return nil, &fileio.PathValidationError{Err: err}
	}
	return openValidated(safePath)
}

// Stat returns file metadata after validating the path.
func (l *LocalFileIO) Stat(name string) (fileio.FileInfo, error) {
	safePath, err := SafeInputPath(name)
	if err != nil {
		return nil, &fileio.PathValidationError{Err: err}
	}
	return vfs.Stat(safePath)
}

// saveResult implements fileio.SaveResult.
type saveResult struct{ size int64 }

func (r *saveResult) Size() int64 { return r.size }

// ResolvePath returns the validated absolute path for the given output path.
func (l *LocalFileIO) ResolvePath(path string) (string, error) {
	resolved, err := SafeOutputPath(path)
	if err != nil {
		return "", &fileio.PathValidationError{Err: err}
	}
	return resolved, nil
}

// Save writes body to path atomically after validating the output path.
// Parent directories are created as needed. The body is streamed directly
// to a temp file and renamed, avoiding full in-memory buffering.
func (l *LocalFileIO) Save(path string, _ fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	safePath, err := SafeOutputPath(path)
	if err != nil {
		return nil, &fileio.PathValidationError{Err: err}
	}
	if err := vfs.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
		return nil, &fileio.MkdirError{Err: err}
	}
	n, err := AtomicWriteFromReader(safePath, body, 0600)
	if err != nil {
		return nil, &fileio.WriteError{Err: err}
	}
	return &saveResult{size: n}, nil
}

// SaveExclusive writes content only when path does not exist, satisfying
// fileio.ExclusiveFileIO. It exists so a no-clobber download policy is enforced
// by the commit itself rather than by an existence check the commit ignores.
func (l *LocalFileIO) SaveExclusive(path string, _ fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	safePath, err := SafeOutputPath(path)
	if err != nil {
		return nil, &fileio.PathValidationError{Err: err}
	}
	if err := vfs.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
		return nil, &fileio.MkdirError{Err: err}
	}
	n, err := ExclusiveWriteFromReader(safePath, body, 0600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		return nil, &fileio.WriteError{Err: err}
	}
	return &saveResult{size: n}, nil
}

// RemoveWorkspaceEntry removes one workspace file or one empty workspace
// directory after applying the same output-path validation as Save.
func (l *LocalFileIO) RemoveWorkspaceEntry(path string) error {
	safePath, err := SafeOutputPath(path)
	if err != nil {
		return &fileio.PathValidationError{Err: err}
	}
	return vfs.Remove(safePath)
}

var _ fileio.ResumableFileIO = (*LocalFileIO)(nil)

// AppendTo opens path (creating it when missing) and streams body onto the end
// of the file, returning the number of bytes written. Bytes already on disk are
// kept on failure so a resumable download can continue from the same offset.
func (l *LocalFileIO) AppendTo(path string, _ fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	safePath, err := SafeOutputPath(path)
	if err != nil {
		return nil, &fileio.PathValidationError{Err: err}
	}
	if err := vfs.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
		return nil, &fileio.MkdirError{Err: err}
	}
	n, err := AppendFromReader(safePath, body, 0600)
	if err != nil {
		return nil, &fileio.WriteError{Err: err}
	}
	return &saveResult{size: n}, nil
}

// ReadResumeArtifact reads one resumable-download artifact after validating
// it as a local input path.
func (l *LocalFileIO) ReadResumeArtifact(path string) ([]byte, error) {
	safePath, err := SafeInputPath(path)
	if err != nil {
		return nil, &fileio.PathValidationError{Err: err}
	}
	return vfs.ReadFile(safePath)
}

// WriteResumeArtifact atomically writes one resumable-download artifact after
// validating it as a local output path.
func (l *LocalFileIO) WriteResumeArtifact(path string, data []byte) error {
	safePath, err := SafeOutputPath(path)
	if err != nil {
		return &fileio.PathValidationError{Err: err}
	}
	if err := vfs.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
		return &fileio.MkdirError{Err: err}
	}
	if err := AtomicWrite(safePath, data, 0600); err != nil {
		return &fileio.WriteError{Err: err}
	}
	return nil
}

// RemoveResumeArtifact removes one resumable-download artifact after applying
// the same output-path policy as Save.
func (l *LocalFileIO) RemoveResumeArtifact(path string) error {
	safePath, err := SafeOutputPath(path)
	if err != nil {
		return &fileio.PathValidationError{Err: err}
	}
	return vfs.Remove(safePath)
}

// CommitResumeArtifact publishes a completed partial file. A no-overwrite
// commit uses a hard link followed by removal of the partial, so an existing
// target cannot be replaced by a race. Overwrite commits remove the target
// first because os.Rename cannot replace an existing file on every supported
// platform.
func (l *LocalFileIO) CommitResumeArtifact(partialPath, targetPath string, overwrite bool) error {
	safePartial, err := SafeOutputPath(partialPath)
	if err != nil {
		return &fileio.PathValidationError{Err: err}
	}
	safeTarget, err := SafeOutputPath(targetPath)
	if err != nil {
		return &fileio.PathValidationError{Err: err}
	}
	if !overwrite {
		if err := vfs.Link(safePartial, safeTarget); err != nil {
			return err
		}
		if err := vfs.Remove(safePartial); err != nil {
			return err
		}
		return nil
	}
	if err := vfs.Remove(safeTarget); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return vfs.Rename(safePartial, safeTarget)
}
