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

// safeIDChars strips path-traversal chars from app IDs.
var safeIDChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// ErrHeld signals retryable contention; callers errors.Is to distinguish from real failures.
var ErrHeld = errors.New("lockfile: lock already held")

type LockFile struct {
	path string
	file *os.File
}

func New(path string) *LockFile {
	return &LockFile{path: path}
}

// ForSubscribe sanitises appID against path traversal before forming the lock filename.
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

// TryLock acquires an exclusive non-blocking lock; auto-released on process exit.
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

// Unlock keeps the file on disk to avoid inode-reuse races between unlock and competing open+flock.
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

func (l *LockFile) Path() string {
	return l.path
}
