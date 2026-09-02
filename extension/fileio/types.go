// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package fileio

import (
	"context"
	"io"
	"io/fs"
)

// Provider creates FileIO instances.
// Follows the same API style as extension/credential.Provider.
type Provider interface {
	Name() string
	ResolveFileIO(ctx context.Context) FileIO
}

// FileIO abstracts file transfer operations for CLI commands.
// The default implementation operates on the local filesystem with
// path validation, directory creation, and atomic writes.
// Inject a custom implementation via Factory.FileIOProvider to replace
// file transfer behavior (e.g. streaming in server mode).
type FileIO interface {
	// Open opens a file for reading (upload, attachment, template scenarios).
	// The default implementation validates the path via SafeInputPath.
	Open(name string) (File, error)

	// Stat returns file metadata (size validation, existence checks).
	// The default implementation validates the path via SafeInputPath.
	// Use os.IsNotExist(err) to distinguish "file not found" from "invalid path".
	Stat(name string) (FileInfo, error)

	// ResolvePath returns the validated, absolute path for the given output path.
	// The default implementation delegates to SafeOutputPath.
	// Use this to obtain the canonical saved path for user-facing output.
	ResolvePath(path string) (string, error)

	// Save writes content to the target path and returns a SaveResult.
	// The default implementation validates via SafeOutputPath, creates
	// parent directories, and writes atomically.
	Save(path string, opts SaveOptions, body io.Reader) (SaveResult, error)
}

// ExclusiveFileIO is an optional extension for providers whose commit step can
// refuse an existing target.
//
// A no-clobber policy cannot be honoured by checking existence and then calling
// Save: another writer may create the target between the two, and the commit
// would overwrite it. Only a provider that makes the commit itself exclusive can
// promise otherwise, so a provider that cannot must leave this interface
// unimplemented -- callers then reject the policy explicitly instead of
// appearing to enforce it.
type ExclusiveFileIO interface {
	FileIO

	// SaveExclusive writes content to path only when path does not exist, and
	// returns an error satisfying errors.Is(err, fs.ErrExist) when it does.
	// A failed write must not leave a partial artifact at path.
	SaveExclusive(path string, opts SaveOptions, body io.Reader) (SaveResult, error)
}

// WorkspaceFileIO is an optional extension for commands that own temporary
// workspace entries. RemoveWorkspaceEntry must remove exactly one file or one
// empty directory, never recursively, and must apply the same path validation
// and storage backend as the embedded FileIO methods.
type WorkspaceFileIO interface {
	FileIO
	RemoveWorkspaceEntry(path string) error
}

// FileInfo is a minimal subset of os.FileInfo covering actual CLI usage.
// os.FileInfo satisfies this interface.
type FileInfo interface {
	Size() int64
	IsDir() bool
	Mode() fs.FileMode
}

// File is the interface returned by FileIO.Open.
// It covers the subset of *os.File methods actually used by CLI commands.
// *os.File satisfies this interface without adaptation.
type File interface {
	io.Reader
	io.ReaderAt
	io.Closer
}

// SaveResult holds the outcome of a Save operation.
type SaveResult interface {
	Size() int64 // actual bytes written
}

// SaveOptions carries metadata for Save.
// The default (local) implementation ignores these fields;
// server-mode implementations use them to construct streaming response frames.
type SaveOptions struct {
	ContentType   string // MIME type
	ContentLength int64  // content length; -1 if unknown
}
