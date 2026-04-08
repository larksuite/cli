package vfs

import (
	"io/fs"
	"os"
)

// OsFs delegates every method to the os standard library.
type OsFs struct{}

// Query
func (OsFs) Stat(name string) (fs.FileInfo, error)  { return os.Stat(name) }
func (OsFs) Lstat(name string) (fs.FileInfo, error) { return os.Lstat(name) }
func (OsFs) Getwd() (string, error)                 { return os.Getwd() }
func (OsFs) UserHomeDir() (string, error)           { return os.UserHomeDir() }

// Read/Write
func (OsFs) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }
func (OsFs) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}
func (OsFs) Open(name string) (*os.File, error) { return os.Open(name) }
func (OsFs) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}
func (OsFs) CreateTemp(dir, pattern string) (*os.File, error) { return os.CreateTemp(dir, pattern) }

// Directory/File management
func (OsFs) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }
func (OsFs) ReadDir(name string) ([]os.DirEntry, error)   { return os.ReadDir(name) }
func (OsFs) Remove(name string) error                     { return os.Remove(name) }
func (OsFs) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
