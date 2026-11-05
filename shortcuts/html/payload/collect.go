// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package payload

import (
	"errors"
	"io/fs"
	"math"
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
	// Via names the file that referenced this one, empty for the entry. A dependency the caller never wrote
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
// keys and create two publications.
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
// Reads share the default raw-payload byte limit; stricter callers use
// CollectFileWithBudget.
func CollectFile(fio fileio.FileIO, relPath string) ([]Candidate, string, []Skip, error) {
	return CollectFileWithBudget(fio, relPath, DefaultLimits().RawTotalBytes)
}

// CollectFileWithBudget collects the same dependency closure as CollectFile,
// but bounds all reads by maxBytes in total, including the entry. Oversized
// dependencies stop collection rather than being silently skipped. The budget
// must be positive and leave room for a one-byte overflow probe.
func CollectFileWithBudget(fio fileio.FileIO, relPath string, maxBytes int64) ([]Candidate, string, []Skip, error) {
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return nil, "", nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "collection byte budget must be positive and less than %d", int64(math.MaxInt64))
	}
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
		maxBytes:  maxBytes,
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
