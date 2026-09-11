// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
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
	// Via names the file that referenced this one, empty for the entry and for
	// everything a --dir walk picks up. A dependency the caller never wrote
	// down is hard to reason about when it turns out to be a problem, so the
	// diagnostics need to be able to say where it came from.
	Via string
}

// isUnsafeRel reports whether a forward-slash relative path must never be
// written into a zip header: absolute, containing a .. component, or holding a
// NUL byte. Component-aware, so names that merely contain ".." as a substring
// (archive.tar..bak) stay allowed.
func isUnsafeRel(rel string) bool {
	// A literal backslash never appears in a path this walker produces
	// (filepath.ToSlash already normalized real separators), so its only
	// source is a file whose name contains one. Reject it: an unpacker that
	// applies Windows semantics would read it as a separator, which is a
	// zip-slip primitive. Defense in depth — the server's unpacker is not
	// ours to verify.
	return strings.Contains(rel, `\`) ||
		strings.HasPrefix(rel, "/") ||
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

// inputPathError re-frames the sandbox rejection that FileIO.Stat returns.
// Left unwrapped it surfaces as an internal error naming --file — a flag this
// command does not have — and suggests reading out-of-tree content from stdin,
// which does not apply to a publish payload. Callers pass their own flag name.
func inputPathError(param, path string, cause error) error {
	// Only claim the path is out of bounds when that is actually what we
	// determined. Earlier revisions used that sentence as the catch-all, so a
	// permission error, a "not a directory" from a bad join, or a symlink loop
	// on a plain ./relative path all told the caller to cd — which changes
	// nothing and, for an agent assembling paths, invites useless retries.
	switch {
	case errors.Is(cause, fs.ErrNotExist):
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"%s %q does not exist", param, path).
			WithParam(param).
			WithCause(cause).
			WithHint("check the path; it is resolved relative to the current directory")

	case errors.Is(cause, fs.ErrPermission):
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"%s %q cannot be read: permission denied", param, path).
			WithParam(param).
			WithCause(cause).
			WithHint("check the permissions on the path and every directory above it")

	case escapesWorkingDir(path):
		// The cause is attached but never interpolated: its text names --file
		// and offers a stdin fallback, neither of which exists here.
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"%s %q is outside the current directory: the path must be relative and must resolve inside it", param, path).
			WithParam(param).
			WithCause(cause).
			WithHint("cd to the directory that holds the payload, then pass a relative path")

	default:
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"%s %q cannot be used", param, path).
			WithParam(param).
			WithCause(cause).
			WithHint("check that the path points at a readable file or directory inside the current directory")
	}
}

// escapesWorkingDir reports whether the input itself is out of bounds — an
// absolute path, or one that climbs above the current directory. Judged from
// the input rather than from the sandbox error so the message only makes this
// claim when it is true.
func escapesWorkingDir(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))
	return cleaned == ".." || strings.HasPrefix(cleaned, "../")
}

// CollectFile resolves a single-file payload: the entry file plus the local
// files it references, transitively. Publishing the page alone would ship a
// document whose stylesheet, scripts and images all 404 -- the artifact would
// be broken, not merely different from what the web client produces.
//
// The walk mirrors the web client's collector, because the publish carries a
// fingerprint of the resulting file set and the GUI compares it against the set
// it would have built itself. Anything the two disagree about surfaces as a
// page the GUI reports as permanently out of sync, with no error to explain it.
//
// relPath goes through the caller's FileIO so the cwd sandbox check runs. The
// entry's resolved absolute path is returned for use as the idempotency key.
// The third return value lists references that were found but not published,
// so the caller can say so instead of silently shipping a page with holes.
func CollectFile(fio fileio.FileIO, relPath string) ([]Candidate, string, []Skip, error) {
	st, err := fio.Stat(relPath)
	if err != nil {
		return nil, "", nil, inputPathError("--file-path", relPath, err)
	}
	if !st.Mode().IsRegular() {
		return nil, "", nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"--file-path %q is not a regular file", relPath).WithParam("--file-path")
	}
	abs, err := canonicalAbs(relPath)
	if err != nil {
		return nil, "", nil, err
	}
	// The payload root is the entry's directory, resolved on its own rather
	// than taken from the entry's own resolved path: if the entry is a symlink,
	// its target lives elsewhere, and dependencies are written relative to
	// where the page sits, not to where the link points.
	rootDir := filepath.Dir(relPath)
	rootAbs, err := canonicalAbs(rootDir)
	if err != nil {
		return nil, "", nil, err
	}
	c := &collector{
		fio:       fio,
		root:      rootDir,
		rootAbs:   rootAbs,
		entryRel:  filepath.Base(relPath),
		validated: map[string]bool{},
	}
	if err := c.walk(); err != nil {
		return nil, "", nil, err
	}
	return c.cands, abs, c.skipped, nil
}

// CollectDir resolves a directory payload. It returns the candidates, the file
// names sitting directly at the directory root (for entry resolution) and the
// directory's absolute path.
func CollectDir(fio fileio.FileIO, relDir string) ([]Candidate, []string, string, error) {
	st, err := fio.Stat(relDir)
	if err != nil {
		return nil, nil, "", inputPathError("--dir", relDir, err)
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
