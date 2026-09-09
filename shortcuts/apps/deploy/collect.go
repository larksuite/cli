// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// Candidate is one file of the payload as collected from disk.
type Candidate struct {
	RelPath string
	AbsPath string
	Size    int64
}

// isUnsafeRel reports whether a forward-slash relative path must never be
// written into a zip header: absolute, containing a .. component, or holding a
// NUL byte. Component-aware, so names that merely contain ".." as a substring
// (archive.tar..bak) stay allowed.
func isUnsafeRel(rel string) bool {
	return strings.HasPrefix(rel, "/") ||
		rel == ".." ||
		strings.HasPrefix(rel, "../") ||
		strings.Contains(rel, "/../") ||
		strings.HasSuffix(rel, "/..") ||
		strings.ContainsRune(rel, 0)
}

// canonicalAbs resolves relPath to an absolute path with symlinks evaluated,
// matching what SafeInputPath produces internally. Plain filepath.Abs is not
// enough: on macOS /tmp is a symlink to /private/tmp, so the same file reached
// through the two spellings would otherwise yield two different idempotency
// keys and create two apps.
func canonicalAbs(relPath string) (string, error) {
	//nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); relPath already passed FileIO.Stat's input validation, and FileIO.ResolvePath validates output paths only.
	abs, err := filepath.Abs(relPath)
	if err != nil {
		return "", errs.NewInternalError(errs.SubtypeFileIO, "resolve %q: %v", relPath, err).WithCause(err)
	}
	//nolint:forbidigo // same rationale as filepath.Abs above; the target exists because the caller already stat-ed it.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", errs.NewInternalError(errs.SubtypeFileIO, "resolve symlinks for %q: %v", relPath, err).WithCause(err)
	}
	return resolved, nil
}

// CollectFile resolves a single-file payload. relPath goes through the caller's
// FileIO so the cwd sandbox check runs; the resolved absolute path is returned
// for use as the idempotency key.
func CollectFile(fio fileio.FileIO, relPath string) ([]Candidate, string, error) {
	st, err := fio.Stat(relPath)
	if err != nil {
		return nil, "", err
	}
	if !st.Mode().IsRegular() {
		return nil, "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"--file-path %q is not a regular file", relPath).WithParam("--file-path")
	}
	abs, err := canonicalAbs(relPath)
	if err != nil {
		return nil, "", err
	}
	name := filepath.Base(relPath)
	return []Candidate{{RelPath: name, AbsPath: relPath, Size: st.Size()}}, abs, nil
}

// CollectDir resolves a directory payload. It returns the candidates, the file
// names sitting directly at the directory root (for entry resolution) and the
// directory's absolute path.
func CollectDir(fio fileio.FileIO, relDir string) ([]Candidate, []string, string, error) {
	st, err := fio.Stat(relDir)
	if err != nil {
		return nil, nil, "", err
	}
	if !st.IsDir() {
		return nil, nil, "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"--dir %q is not a directory; use --file-path to publish a single file", relDir).WithParam("--dir")
	}
	cands, rootNames, _, err := collectDirAt(relDir)
	if err != nil {
		return nil, nil, "", err
	}
	abs, err := canonicalAbs(relDir)
	if err != nil {
		return nil, nil, "", err
	}
	return cands, rootNames, abs, nil
}

// collectDirAt walks root and returns every regular file as a candidate.
// Symlinks are not followed and a .git entry skips its whole subtree.
func collectDirAt(root string) ([]Candidate, []string, string, error) {
	var cands []Candidate
	var rootNames []string
	//nolint:forbidigo // the repository forbids direct filesystem calls, but fileio exposes no WalkDir; root is validated by the caller's fio.Stat.
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errs.NewInternalError(errs.SubtypeFileIO, "walk %q: %v", path, walkErr).WithCause(walkErr)
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return errs.NewInternalError(errs.SubtypeFileIO, "stat %q: %v", path, err).WithCause(err)
		}
		// Regular files only: symlinks, devices, pipes and sockets are skipped
		// so a link cannot pull content from outside the payload root.
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return errs.NewInternalError(errs.SubtypeFileIO, "relativize %q: %v", path, err).WithCause(err)
		}
		relSlash := filepath.ToSlash(rel)
		if isUnsafeRel(relSlash) {
			return errs.NewInternalError(errs.SubtypeUnknown, "unsafe relative path %q for %s", relSlash, path)
		}
		if !strings.Contains(relSlash, "/") {
			rootNames = append(rootNames, relSlash)
		}
		cands = append(cands, Candidate{RelPath: relSlash, AbsPath: path, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, nil, "", err
	}
	return cands, rootNames, root, nil
}
