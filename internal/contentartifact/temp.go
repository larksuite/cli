// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package contentartifact saves oversized command output to local files.
package contentartifact

import (
	"io"
	"io/fs"
	"path/filepath"

	"github.com/larksuite/cli/internal/vfs"
)

const tempMarkdownPattern = "lark-cli-fetch-*.md"

type tempFile interface {
	Name() string
	Chmod(mode fs.FileMode) error
	Write(p []byte) (int, error)
	Sync() error
	Close() error
}

// WriteTempMarkdown writes content exactly as provided to a private file in the
// platform temporary directory. The caller owns the returned file and may
// remove it when it is no longer needed.
func WriteTempMarkdown(content []byte) (absolutePath string, size int64, err error) {
	return writeTempMarkdown(
		content,
		func(dir, pattern string) (tempFile, error) {
			return vfs.CreateTemp(dir, pattern)
		},
		vfs.Getwd,
		vfs.Remove,
	)
}

func writeTempMarkdown(
	content []byte,
	createTemp func(dir, pattern string) (tempFile, error),
	getwd func() (string, error),
	remove func(string) error,
) (string, int64, error) {
	tmp, err := createTemp("", tempMarkdownPattern)
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()

	closed := false
	succeeded := false
	defer func() {
		if succeeded {
			return
		}
		if !closed {
			_ = tmp.Close()
		}
		_ = remove(tmpName)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return "", 0, err
	}
	written, err := tmp.Write(content)
	if err != nil {
		return "", 0, err
	}
	if written != len(content) {
		return "", 0, io.ErrShortWrite
	}
	if err := tmp.Sync(); err != nil {
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	closed = true

	absolutePath := tmpName
	if !filepath.IsAbs(absolutePath) {
		cwd, err := getwd()
		if err != nil {
			return "", 0, err
		}
		absolutePath = filepath.Join(cwd, absolutePath)
	}
	absolutePath = filepath.Clean(absolutePath)

	succeeded = true
	return absolutePath, int64(written), nil
}
