// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lockfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs"
)

// safeIDChars strips everything except alphanumerics, underscores, hyphens, and dots
// to prevent path traversal via crafted app IDs (e.g. "../../tmp/evil").
var safeIDChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// ErrHeld is returned (wrapped) when TryLock cannot acquire the lock because
// another owner holds it — either this same LockFile instance was already
// locked, or another process holds the flock (Unix) / LockFileEx (Windows)
// on the underlying file. Callers use errors.Is(err, ErrHeld) to distinguish
// benign contention (retryable) from real failures (e.g. EACCES, missing
// parent dir) which should surface.
var ErrHeld = errors.New("lockfile: lock already held")

// LockFile represents an exclusive file lock.
type LockFile struct {
	path string
	file *os.File
}

// New creates a LockFile for the given path (does not acquire the lock).
func New(path string) *LockFile {
	return &LockFile{path: path}
}

// ForSubscribe returns a LockFile scoped to the event subscribe command for a given App ID.
// Lock path: {configDir}/locks/subscribe_{appID}.lock
//
// The appID is sanitized to prevent path traversal: any character outside
// [a-zA-Z0-9._-] is replaced with "_", and filepath.Base strips directory
// components, so a malicious appID like "../../tmp/evil" becomes a flat
// filename under the locks directory.
func ForSubscribe(appID string) (*LockFile, error) {
	if appID == "" {
		return nil, fmt.Errorf("app ID must not be empty")
	}
	dir := filepath.Join(core.GetConfigDir(), "locks")
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	safe := safeIDChars.ReplaceAllString(appID, "_")
	name := filepath.Base(fmt.Sprintf("subscribe_%s.lock", safe))
	path := filepath.Join(dir, name)
	return New(path), nil
}

// TryLock attempts to acquire an exclusive, non-blocking lock.
// Returns nil on success. Returns an error if the lock is already held
// by another process (or on any other failure).
// The lock is automatically released when the process exits.
func (l *LockFile) TryLock() error {
	if l.file != nil {
		return fmt.Errorf("%w: %s", ErrHeld, l.path)
	}
	f, err := vfs.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	if err := tryLockFile(f); err != nil {
		f.Close()
		return err
	}
	l.file = f
	return nil
}

// Unlock releases the lock and closes the file descriptor.
// The lock file is intentionally kept on disk to avoid an inode-reuse race:
// removing the path between unlock and a competing open+flock would let two
// processes lock different inodes under the same name.
func (l *LockFile) Unlock() error {
	if l.file == nil {
		return nil
	}
	err := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return fmt.Errorf("unlock file: %w", err)
	}
	return closeErr
}

// Path returns the lock file path.
func (l *LockFile) Path() string {
	return l.path
}
