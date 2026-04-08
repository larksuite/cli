package vfs

import (
	"io/fs"
	"os"
)

// DefaultFS is the global filesystem instance used by business code.
// It points to the real OS implementation; tests may replace it with a mock.
var DefaultFS FS = OsFs{}

// Package-level convenience functions that delegate to DefaultFS.

func Stat(name string) (fs.FileInfo, error)  { return DefaultFS.Stat(name) }
func Lstat(name string) (fs.FileInfo, error) { return DefaultFS.Lstat(name) }
func Getwd() (string, error)                 { return DefaultFS.Getwd() }
func UserHomeDir() (string, error)           { return DefaultFS.UserHomeDir() }
func ReadFile(name string) ([]byte, error)   { return DefaultFS.ReadFile(name) }
func WriteFile(name string, data []byte, perm fs.FileMode) error {
	return DefaultFS.WriteFile(name, data, perm)
}
func Open(name string) (*os.File, error) { return DefaultFS.Open(name) }
func OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return DefaultFS.OpenFile(name, flag, perm)
}
func CreateTemp(dir, pattern string) (*os.File, error) { return DefaultFS.CreateTemp(dir, pattern) }
func MkdirAll(path string, perm fs.FileMode) error     { return DefaultFS.MkdirAll(path, perm) }
func ReadDir(name string) ([]os.DirEntry, error)       { return DefaultFS.ReadDir(name) }
func Remove(name string) error                         { return DefaultFS.Remove(name) }
func Rename(oldpath, newpath string) error             { return DefaultFS.Rename(oldpath, newpath) }
