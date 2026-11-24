// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package workflowrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

// RequireLocal refuses portable providers: fsync/rename/locking require a host
// tree and cannot be emulated by the public streaming FileIO interface.
func RequireLocal(f fileio.FileIO) error {
	if _, ok := f.(*localfileio.LocalFileIO); ok {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "mock export requires the local FileIO provider")
}

type Store struct {
	Root string
	lock *flock.Flock
	Hook func(string, Batch) error
}

func ioError(e error) error {
	if e == nil {
		return nil
	}
	if errs.IsTyped(e) {
		return e
	}
	return errs.NewInternalError(errs.SubtypeFileIO, "workflow log store operation failed").WithCause(e)
}
func safeTree(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "export root must be a resolved absolute path")
	}
	for cur := root; ; cur = filepath.Dir(cur) {
		i, e := vfs.Lstat(cur)
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return ioError(e)
		}
		if e == nil && i.Mode()&fs.ModeSymlink != 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "export root contains a symlink")
		}
		if filepath.Dir(cur) == cur {
			break
		}
	}
	return nil
}
func OpenStore(root string, resume bool) (*Store, error) {
	if e := safeTree(root); e != nil {
		return nil, e
	}
	if e := vfs.MkdirAll(root, 0700); e != nil {
		return nil, ioError(e)
	}
	// flock serializes writers to this validated local-only export directory.
	s := &Store{Root: root, lock: flock.New(filepath.Join(root, ".lock"))}
	ok, e := s.lock.TryLock()
	if e != nil {
		return nil, ioError(e)
	}
	if !ok {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition, "export directory is already occupied")
	}
	if !resume {
		entries, e := vfs.ReadDir(root)
		if e != nil {
			s.Close()
			return nil, ioError(e)
		}
		for _, i := range entries {
			if i.Name() != ".lock" {
				s.Close()
				return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition, "new export requires an empty directory; use --resume")
			}
		}
	}
	for _, d := range []string{Runs, Details, "button-events", "merged", "pending", "pending/orphans"} {
		path := filepath.Join(root, d)
		if e := safeTree(path); e != nil {
			s.Close()
			return nil, e
		}
		if e := vfs.MkdirAll(path, 0700); e != nil {
			s.Close()
			return nil, ioError(e)
		}
	}
	if e := syncDir(root); e != nil {
		s.Close()
		return nil, ioError(e)
	}
	if e := syncDir(filepath.Dir(root)); e != nil {
		s.Close()
		return nil, ioError(e)
	}
	return s, nil
}
func (s *Store) Close() {
	if s.lock != nil {
		_ = s.lock.Unlock()
	}
}
func (s *Store) path(rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || filepath.Clean(rel) != rel || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe store entry")
	}
	p := filepath.Join(s.Root, rel)
	if e := safeTree(p); e != nil {
		return "", e
	}
	return p, nil
}
func syncDir(path string) error {
	f, e := vfs.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (s *Store) atomic(rel string, b []byte, immutable bool) error {
	p, e := s.path(rel)
	if e != nil {
		return e
	}
	if immutable {
		if _, e := vfs.Lstat(p); e == nil {
			return errs.NewInternalError(errs.SubtypeFileIO, "immutable shard already exists")
		} else if !errors.Is(e, fs.ErrNotExist) {
			return ioError(e)
		}
	}
	part := p + ".part"
	// All entries have validated, CLI-generated names, under an exclusively
	// locked local directory. O_EXCL prevents following pre-existing part files.
	f, e := vfs.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return ioError(e)
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return ioError(e)
	}
	if e = vfs.Rename(part, p); e != nil {
		return ioError(e)
	}
	return ioError(syncDir(filepath.Dir(p)))
}
func (s *Store) SaveManifest(m *Manifest) error {
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return ioError(e)
	}
	return s.atomic("manifest.json", b, false)
}
func (s *Store) ReadManifest() (*Manifest, error) {
	p, e := s.path("manifest.json")
	if e != nil {
		return nil, e
	}
	b, e := vfs.ReadFile(p)
	if e != nil {
		return nil, ioError(e)
	}
	var m Manifest
	if e = json.Unmarshal(b, &m); e != nil {
		return nil, ioError(e)
	}
	return &m, nil
}
func folder(plane string) string {
	if plane == Buttons {
		return "button-events"
	}
	return plane
}
func shardName(b Batch) string {
	return fmt.Sprintf("%s/%s-%06d.json", folder(b.Page.Plane), b.JobID, b.Seq)
}
func ckptName(b Batch) string {
	return fmt.Sprintf("pending/ckpt-%s-%s-%06d.json", b.Page.Plane, b.JobID, b.Seq)
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (s *Store) checkpoint(b Batch, data []byte) error {
	c := Checkpoint{b, shardName(b), digest(data)}
	wire, e := json.Marshal(c)
	if e != nil {
		return ioError(e)
	}
	return s.atomic(ckptName(b), wire, true)
}
func (s *Store) Commit(b Batch) error {
	wire, e := json.Marshal(b)
	if e != nil {
		return ioError(e)
	}
	if e = s.atomic(shardName(b), wire, true); e != nil {
		return e
	}
	if s.Hook != nil {
		if e = s.Hook("data_durable", b); e != nil {
			return e
		}
	}
	if e = s.checkpoint(b, wire); e != nil {
		return e
	}
	if s.Hook != nil {
		if e = s.Hook("checkpoint_durable", b); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) Checkpoints() ([]Checkpoint, error) {
	dir, e := s.path("pending")
	if e != nil {
		return nil, e
	}
	entries, e := vfs.ReadDir(dir)
	if e != nil {
		return nil, ioError(e)
	}
	out := []Checkpoint{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "ckpt-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		p, e := s.path("pending/" + entry.Name())
		if e != nil {
			return nil, e
		}
		wire, e := vfs.ReadFile(p)
		if e != nil {
			return nil, ioError(e)
		}
		var c Checkpoint
		if e = json.Unmarshal(wire, &c); e != nil {
			return nil, ioError(e)
		}
		if c.Shard != shardName(c.Batch) || "pending/"+entry.Name() != ckptName(c.Batch) {
			return nil, errs.NewInternalError(errs.SubtypeFileIO, "checkpoint entry does not match batch")
		}
		shard, e := s.path(c.Shard)
		if e != nil {
			return nil, e
		}
		b, e := vfs.ReadFile(shard)
		if e != nil {
			return nil, ioError(e)
		}
		expected, _ := json.Marshal(c.Batch)
		if digest(b) != c.SHA256 || string(b) != string(expected) {
			return nil, errs.NewInternalError(errs.SubtypeFileIO, "checkpoint shard integrity check failed")
		}
		out = append(out, c)
	}
	sort.Slice(out, func(a, b int) bool {
		x, y := out[a].Batch, out[b].Batch
		if x.JobID == y.JobID {
			return x.Seq < y.Seq
		}
		return x.JobID < y.JobID
	})
	return out, nil
}
func apply(j *Job, b Batch) error {
	expected, actual := j.Query, b.Query
	expected.PageSize = 0
	actual.PageSize = 0
	expectedWire, _ := json.Marshal(expected)
	actualWire, _ := json.Marshal(actual)
	if string(expectedWire) != string(actualWire) {
		return errs.NewInternalError(errs.SubtypeFileIO, "batch query conflicts with frozen job")
	}
	if b.Seq != j.Seq+1 || b.Previous != j.Query.Token || b.Query.Run != j.Query.Run || b.Query.Window != j.Query.Window || b.Page.Plane != j.Plane || b.Page.Window != j.Query.Window.Wire() {
		return errs.NewInternalError(errs.SubtypeFileIO, "batch boundary is not continuous")
	}
	if b.Page.Result == "" || b.FetchedAt == 0 || (b.Page.HasMore && (b.Page.Token == "" || b.Page.Token == b.Previous)) {
		return errs.NewInternalError(errs.SubtypeFileIO, "batch has no verifiable termination/cursor metadata")
	}
	j.Seq = b.Seq
	j.Query.Token = b.Page.Token
	j.Query.PageSize = 0
	if j.Coverage == "" {
		j.Coverage = Guaranteed
	}
	if b.Page.Coverage != Guaranteed {
		j.Coverage = b.Page.Coverage
		if j.Coverage == "" {
			j.Coverage = Unverified
		}
	}
	j.Uncovered = append(j.Uncovered, b.Page.Uncovered...)
	j.Done = !b.Page.HasMore
	switch b.Page.Result {
	case "data_expired":
		j.Reasons = appendUnique(j.Reasons, "data_expired")
		j.Done = true
	case "permission_denied":
		j.Reasons = appendUnique(j.Reasons, "permission_denied")
		j.Done = false
	case "truncated":
		j.Reasons = appendUnique(j.Reasons, "server_clip")
	}
	if b.Page.Coverage != Guaranteed && b.Page.Result != "data_expired" {
		j.Reasons = appendUnique(j.Reasons, "coverage_unverified")
	}
	if j.Plane == Buttons && (!b.Page.Caps.CompositeCursor || (b.Page.Caps.Attribution != ResponseField && b.Page.Caps.Attribution != StrictLegacy)) {
		j.Reasons = appendUnique(j.Reasons, "button_pagination_not_exactly_once")
	}
	return nil
}
func (s *Store) Recover(m *Manifest) error {
	ckpts, e := s.Checkpoints()
	if e != nil {
		return e
	}
	known := map[string]bool{}
	// Replay all checkpoints, including earlier windows, rather than trusting
	// the last manifest write (which can lag a durable checkpoint).
	for n := range m.Jobs {
		m.Jobs[n].Seq = 0
		m.Jobs[n].Query.Token = ""
		m.Jobs[n].Done = false
		m.Jobs[n].Coverage = ""
		m.Jobs[n].Reasons = nil
		m.Jobs[n].Uncovered = nil
	}
	for _, c := range ckpts {
		known[c.Shard] = true
		found := false
		for n := range m.Jobs {
			if m.Jobs[n].ID == c.Batch.JobID {
				if e = apply(&m.Jobs[n], c.Batch); e != nil {
					return e
				}
				found = true
				break
			}
		}
		if !found {
			return errs.NewInternalError(errs.SubtypeFileIO, "checkpoint references an unknown job")
		}
	}
	for _, dir := range []string{"pending", Runs, Details, "button-events", "merged"} {
		path, e := s.path(dir)
		if e != nil {
			return e
		}
		entries, e := vfs.ReadDir(path)
		if e != nil {
			return ioError(e)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			rel := dir + "/" + entry.Name()
			isPart := strings.HasSuffix(rel, ".part")
			isShard := dir != "pending" && dir != "merged" && strings.HasSuffix(rel, ".json")
			if !isPart && (!isShard || known[rel]) {
				continue
			}
			p, e := s.path(rel)
			if e != nil {
				return e
			}
			if isPart && (dir == "pending" || dir == "merged") {
				if e = vfs.Remove(p); e != nil {
					return ioError(e)
				}
				if e = syncDir(path); e != nil {
					return ioError(e)
				}
				continue
			}
			if isShard && !isPart {
				data, e := vfs.ReadFile(p)
				if e != nil {
					return ioError(e)
				}
				var b Batch
				if json.Unmarshal(data, &b) == nil && rel == shardName(b) {
					for n := range m.Jobs {
						j := &m.Jobs[n]
						if j.ID != b.JobID {
							continue
						}
						trial := *j
						trial.Reasons = append([]string(nil), j.Reasons...)
						if apply(&trial, b) == nil {
							if e = s.checkpoint(b, data); e != nil {
								return e
							}
							*j = trial
							known[rel] = true
							break
						}
					}
					if known[rel] {
						continue
					}
				}
			}
			dest, e := s.path("pending/orphans/" + dir + "-" + entry.Name())
			if e != nil {
				return e
			}
			if _, e = vfs.Lstat(dest); e == nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "orphan quarantine entry already exists")
			}
			if e = vfs.Rename(p, dest); e != nil {
				return ioError(e)
			}
			if e = syncDir(path); e != nil {
				return ioError(e)
			}
			if e = syncDir(filepath.Dir(dest)); e != nil {
				return ioError(e)
			}
		}
	}
	// A failed manifest commit is never authoritative; checkpoint replay is.
	p, _ := s.path("manifest.json.part")
	if e = vfs.Remove(p); e != nil && !errors.Is(e, fs.ErrNotExist) {
		return ioError(e)
	}
	return nil
}
func (s *Store) Publish(m *Manifest) error {
	ckpts, e := s.Checkpoints()
	if e != nil {
		return e
	}
	for _, plane := range m.Params.Planes {
		all := true
		for _, j := range m.Jobs {
			if j.Plane == plane && !j.Done {
				all = false
			}
		}
		if !all {
			continue
		}
		type seen struct {
			item Item
			time int64
			seq  int
		}
		merged := map[string]seen{}
		for _, c := range ckpts {
			b := c.Batch
			if b.Page.Plane != plane {
				continue
			}
			for _, i := range b.Page.Items {
				k := Key(plane, i)
				old, ok := merged[k]
				if !ok || b.FetchedAt > old.time || (b.FetchedAt == old.time && b.Seq > old.seq) {
					merged[k] = seen{i, b.FetchedAt, b.Seq}
				}
			}
		}
		keys := make([]string, 0, len(merged))
		for k := range merged {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var wire strings.Builder
		for _, k := range keys {
			b, e := json.Marshal(merged[k].item)
			if e != nil {
				return ioError(e)
			}
			wire.Write(b)
			wire.WriteByte('\n')
		}
		if e = s.atomic("merged/"+folder(plane)+".ndjson", []byte(wire.String()), false); e != nil {
			return e
		}
	}
	return nil
}
