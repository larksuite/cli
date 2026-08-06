// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillpolicy

import (
	"io"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// overlayFS layers an upper skill tree over a lower one at skill (top
// path segment) granularity: a skill present in upper is served wholly
// from upper, Remove/Allow filter the lower tree, and everything else falls
// through to lower. It is the runtime form of a SkillsOverlay's
// Base -> Allow -> Remove -> Overlay composition.
//
// It implements the io/fs fast-path interfaces (ReadDir/Stat/ReadFile)
// so the io/fs helpers route through the merge instead of hitting a
// single underlying tree.
type overlayFS struct {
	// The manifest: top-level skill name -> owning tree and integrity
	// metadata, snapshotted once at composition. Routing, listing, and
	// dependency validation consult the same snapshot, so a top-level
	// directory added later cannot appear through only one of those surfaces.
	// Contents WITHIN a skill directory are still read live.
	owner   map[string]skillOwner
	entries []fs.DirEntry // manifest listing, sorted by name
}

type skillOwner struct {
	source   fs.FS
	manifest skillManifest
}

var (
	_ fs.FS         = (*overlayFS)(nil)
	_ fs.ReadDirFS  = (*overlayFS)(nil)
	_ fs.StatFS     = (*overlayFS)(nil)
	_ fs.ReadFileFS = (*overlayFS)(nil)
)

func newOverlayFS(lower, upper skillTreeSnapshot, remove, allow []string) *overlayFS {
	removed := make(map[string]bool, len(remove))
	for _, r := range remove {
		removed[r] = true
	}
	var allowed map[string]bool
	if len(allow) > 0 {
		allowed = make(map[string]bool, len(allow))
		for _, a := range allow {
			allowed[a] = true
		}
	}

	o := &overlayFS{owner: map[string]skillOwner{}}
	for name, manifest := range upper.skills {
		o.owner[name] = skillOwner{source: upper.source, manifest: manifest}
	}
	for name, manifest := range lower.skills {
		if removed[name] {
			continue
		}
		if _, shadowed := o.owner[name]; shadowed {
			continue
		}
		if allowed != nil && !allowed[name] {
			continue
		}
		o.owner[name] = skillOwner{source: lower.source, manifest: manifest}
	}

	// Derive the listing from the routing manifest after composition. Keeping
	// owner as the single writable source prevents list/read divergence if a
	// future branch forgets to update a parallel entries collection.
	o.entries = make([]fs.DirEntry, 0, len(o.owner))
	for name := range o.owner {
		o.entries = append(o.entries, skillDirEntry(name))
	}
	sort.Slice(o.entries, func(i, j int) bool { return o.entries[i].Name() < o.entries[j].Name() })
	return o
}

// route picks the tree owning name by its top path segment via the
// composition-time manifest. whiteout is true when the manifest does not
// carry the skill (removed, filtered by Allow, or added after composition).
func (o *overlayFS) route(name string) (target fs.FS, whiteout bool) {
	top := name
	if i := strings.IndexByte(name, '/'); i >= 0 {
		top = name[:i]
	}
	if t, ok := o.owner[top]; ok {
		return t.source, false
	}
	return nil, true
}

func (o *overlayFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		entries, err := o.ReadDir(".")
		if err != nil {
			return nil, err
		}
		return &rootDir{entries: entries}, nil
	}
	target, whiteout := o.route(name)
	if whiteout || target == nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return target.Open(name)
}

func (o *overlayFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		return rootInfo{}, nil
	}
	target, whiteout := o.route(name)
	if whiteout || target == nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return fs.Stat(target, name)
}

func (o *overlayFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) || name == "." {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	target, whiteout := o.route(name)
	if whiteout || target == nil {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}
	return fs.ReadFile(target, name)
}

func (o *overlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	if name != "." {
		target, whiteout := o.route(name)
		if whiteout || target == nil {
			return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
		}
		return fs.ReadDir(target, name)
	}
	return o.mergedRoot()
}

// mergedRoot serves the composition-time manifest listing.
func (o *overlayFS) mergedRoot() ([]fs.DirEntry, error) {
	out := make([]fs.DirEntry, len(o.entries))
	copy(out, o.entries)
	return out, nil
}

// skillDirEntry is the immutable top-level directory entry captured during
// composition. Never retain a plugin FS's fs.DirEntry: custom ReadDirFS
// implementations may return aliased, mutable entries and would otherwise be
// able to change the validated manifest after Resolve returns.
type skillDirEntry string

func (e skillDirEntry) Name() string               { return string(e) }
func (skillDirEntry) IsDir() bool                  { return true }
func (skillDirEntry) Type() fs.FileMode            { return fs.ModeDir }
func (e skillDirEntry) Info() (fs.FileInfo, error) { return skillDirInfo(e), nil }

type skillDirInfo string

func (i skillDirInfo) Name() string     { return string(i) }
func (skillDirInfo) Size() int64        { return 0 }
func (skillDirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o555 }
func (skillDirInfo) ModTime() time.Time { return time.Time{} }
func (skillDirInfo) IsDir() bool        { return true }
func (skillDirInfo) Sys() any           { return nil }

// rootInfo is the synthetic FileInfo for the overlay's "." directory,
// which has no single backing entry in either tree.
type rootInfo struct{}

func (rootInfo) Name() string       { return "." }
func (rootInfo) Size() int64        { return 0 }
func (rootInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o555 }
func (rootInfo) ModTime() time.Time { return time.Time{} }
func (rootInfo) IsDir() bool        { return true }
func (rootInfo) Sys() any           { return nil }

// rootDir is the synthetic directory file returned by Open(".").
type rootDir struct {
	entries []fs.DirEntry
	off     int
}

func (d *rootDir) Stat() (fs.FileInfo, error) { return rootInfo{}, nil }
func (d *rootDir) Close() error               { return nil }

func (d *rootDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: ".", Err: fs.ErrInvalid}
}

func (d *rootDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		rest := d.entries[d.off:]
		d.off = len(d.entries)
		return rest, nil
	}
	if d.off >= len(d.entries) {
		return nil, io.EOF
	}
	end := d.off + n
	if end > len(d.entries) {
		end = len(d.entries)
	}
	part := d.entries[d.off:end]
	d.off = end
	return part, nil
}
