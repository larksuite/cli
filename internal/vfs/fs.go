// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vfs

import (
	"io/fs"
	"os"
)

// FS abstracts filesystem operations used across the project.
// Implementations must behave identically to the corresponding os package functions.
type FS interface {
	// Query
	Stat(name string) (fs.FileInfo, error)
	Lstat(name string) (fs.FileInfo, error)
	Getwd() (string, error)
	UserHomeDir() (string, error)

	// Read/Write
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	Open(name string) (*os.File, error)
	OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error)
	CreateTemp(dir, pattern string) (*os.File, error)

	// Directory/File management
	MkdirAll(path string, perm fs.FileMode) error
	ReadDir(name string) ([]os.DirEntry, error)
	Remove(name string) error
	Rename(oldpath, newpath string) error
}
