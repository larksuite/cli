// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillpolicy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"
)

// pluginFS is the trust boundary around plugin-provided distribution assets.
// A third-party fs.FS is executable Go code: every method, including methods
// on returned files and directory entries, may panic. Convert those panics to
// ordinary path errors so startup and runtime reads remain diagnosable.
type pluginFS struct {
	owner  string
	field  string
	source fs.FS
}

var (
	_ fs.FS         = (*pluginFS)(nil)
	_ fs.ReadDirFS  = (*pluginFS)(nil)
	_ fs.ReadFileFS = (*pluginFS)(nil)
	_ fs.StatFS     = (*pluginFS)(nil)
)

func protectPluginFS(owner, field string, source fs.FS) fs.FS {
	if source == nil {
		return nil
	}
	return &pluginFS{owner: owner, field: field, source: source}
}

func (p *pluginFS) Open(name string) (file fs.File, err error) {
	defer p.recoverPath("open", name, &err)
	file, err = p.source.Open(name)
	if err != nil {
		return nil, p.pathError("open", name, err)
	}
	if file == nil {
		return nil, p.pathError("open", name, errorsNilResult)
	}
	safe := &pluginFile{fsys: p, path: name, source: file}
	if dir, ok := file.(fs.ReadDirFile); ok {
		return &pluginReadDirFile{pluginFile: safe, dir: dir}, nil
	}
	return safe, nil
}

func (p *pluginFS) Stat(name string) (info fs.FileInfo, err error) {
	defer func() {
		if value := recover(); value != nil {
			info = nil
			err = p.pathError("stat", name, fmt.Errorf("panic: %v", value))
		}
	}()
	info, err = fs.Stat(p.source, name)
	if err != nil {
		return nil, p.pathError("stat", name, err)
	}
	if info == nil {
		return nil, p.pathError("stat", name, errorsNilResult)
	}
	return snapshotFileInfo(info), nil
}

func (p *pluginFS) ReadFile(name string) (data []byte, err error) {
	defer p.recoverPath("readfile", name, &err)
	data, err = fs.ReadFile(p.source, name)
	if err != nil {
		return nil, p.pathError("readfile", name, err)
	}
	return data, nil
}

func (p *pluginFS) ReadDir(name string) (entries []fs.DirEntry, err error) {
	defer func() {
		if value := recover(); value != nil {
			entries = nil
			err = p.pathError("readdir", name, fmt.Errorf("panic: %v", value))
		}
	}()
	entries, err = fs.ReadDir(p.source, name)
	if err != nil {
		return nil, p.pathError("readdir", name, err)
	}
	return p.protectEntries(name, entries)
}

func (p *pluginFS) protectEntries(path string, entries []fs.DirEntry) ([]fs.DirEntry, error) {
	safe := make([]fs.DirEntry, len(entries))
	for i, entry := range entries {
		if entry == nil {
			return nil, p.pathError("readdir", path, errorsNilResult)
		}
		// Name, IsDir, and Type cannot report errors, so snapshot them while
		// the enclosing ReadDir recovery boundary is active.
		name := entry.Name()
		safe[i] = &pluginDirEntry{
			fsys:   p,
			path:   joinPath(path, name),
			source: entry,
			name:   name,
			isDir:  entry.IsDir(),
			typ:    entry.Type(),
		}
	}
	return safe, nil
}

func (p *pluginFS) recoverPath(op, path string, err *error) {
	if value := recover(); value != nil {
		*err = p.pathError(op, path, fmt.Errorf("panic: %v", value))
	}
}

func (p *pluginFS) pathError(op, path string, cause error) error {
	return &fs.PathError{
		Op:   op,
		Path: path,
		Err:  fmt.Errorf("plugin %q %s filesystem: %w", p.owner, p.field, cause),
	}
}

var errorsNilResult = fmt.Errorf("returned nil without an error")

func joinPath(parent, child string) string {
	if parent == "." {
		return child
	}
	return parent + "/" + child
}

type pluginFile struct {
	fsys   *pluginFS
	path   string
	source fs.File
}

func (f *pluginFile) Stat() (info fs.FileInfo, err error) {
	defer func() {
		if value := recover(); value != nil {
			info = nil
			err = f.fsys.pathError("stat", f.path, fmt.Errorf("panic: %v", value))
		}
	}()
	info, err = f.source.Stat()
	if err != nil {
		return nil, f.fsys.pathError("stat", f.path, err)
	}
	if info == nil {
		return nil, f.fsys.pathError("stat", f.path, errorsNilResult)
	}
	return snapshotFileInfo(info), nil
}

func (f *pluginFile) Read(buffer []byte) (n int, err error) {
	defer f.fsys.recoverPath("read", f.path, &err)
	n, err = f.source.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, f.fsys.pathError("read", f.path, err)
	}
	return n, err
}

func (f *pluginFile) Close() (err error) {
	defer f.fsys.recoverPath("close", f.path, &err)
	if err = f.source.Close(); err != nil {
		return f.fsys.pathError("close", f.path, err)
	}
	return nil
}

type pluginReadDirFile struct {
	*pluginFile
	dir fs.ReadDirFile
}

func (f *pluginReadDirFile) ReadDir(count int) (entries []fs.DirEntry, err error) {
	defer func() {
		if value := recover(); value != nil {
			entries = nil
			err = f.fsys.pathError("readdir", f.path, fmt.Errorf("panic: %v", value))
		}
	}()
	entries, err = f.dir.ReadDir(count)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, f.fsys.pathError("readdir", f.path, err)
	}
	safe, protectErr := f.fsys.protectEntries(f.path, entries)
	if protectErr != nil {
		return nil, protectErr
	}
	return safe, err
}

type pluginDirEntry struct {
	fsys   *pluginFS
	path   string
	source fs.DirEntry
	name   string
	isDir  bool
	typ    fs.FileMode
}

func (e *pluginDirEntry) Name() string      { return e.name }
func (e *pluginDirEntry) IsDir() bool       { return e.isDir }
func (e *pluginDirEntry) Type() fs.FileMode { return e.typ }

func (e *pluginDirEntry) Info() (info fs.FileInfo, err error) {
	defer func() {
		if value := recover(); value != nil {
			info = nil
			err = e.fsys.pathError("stat", e.path, fmt.Errorf("panic: %v", value))
		}
	}()
	info, err = e.source.Info()
	if err != nil {
		return nil, e.fsys.pathError("stat", e.path, err)
	}
	if info == nil {
		return nil, e.fsys.pathError("stat", e.path, errorsNilResult)
	}
	return snapshotFileInfo(info), nil
}

type pluginFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
	sys     any
}

func snapshotFileInfo(info fs.FileInfo) fs.FileInfo {
	return pluginFileInfo{
		name:    info.Name(),
		size:    info.Size(),
		mode:    info.Mode(),
		modTime: info.ModTime(),
		isDir:   info.IsDir(),
		sys:     info.Sys(),
	}
}

func (i pluginFileInfo) Name() string       { return i.name }
func (i pluginFileInfo) Size() int64        { return i.size }
func (i pluginFileInfo) Mode() fs.FileMode  { return i.mode }
func (i pluginFileInfo) ModTime() time.Time { return i.modTime }
func (i pluginFileInfo) IsDir() bool        { return i.isDir }
func (i pluginFileInfo) Sys() any           { return i.sys }
