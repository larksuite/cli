// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	tokenStorageLockTimeout    = 30 * time.Second
	tokenStorageLockRetryDelay = 500 * time.Millisecond
)

var tokenStorageProcessLocks sync.Map

var safeIDChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// sanitizeID replaces characters that are unsafe in lock filenames.
func sanitizeID(id string) string {
	return safeIDChars.ReplaceAllString(id, "_")
}

func tokenStorageLockDir() string {
	return filepath.Join(core.GetBaseConfigDir(), "locks")
}

func tokenStorageLockPath(appID, userOpenID string) string {
	return filepath.Join(tokenStorageLockDir(), fmt.Sprintf("refresh_%s_%s.lock",
		sanitizeID(appID), sanitizeID(userOpenID)))
}

func tokenStorageProcessLock(appID, userOpenID string) *sync.Mutex {
	key := accountKey(appID, userOpenID)
	lock, _ := tokenStorageProcessLocks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// withTokenStorageLock runs fn while holding both the process-local and
// cross-process locks for one account.
func withTokenStorageLock(appID, userOpenID string, fn func() error) (err error) {
	processLock := tokenStorageProcessLock(appID, userOpenID)
	processLock.Lock()
	defer processLock.Unlock()

	lockDir := tokenStorageLockDir()
	if err := vfs.MkdirAll(lockDir, 0700); err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO,
			"failed to prepare token storage lock").
			WithCause(err).
			WithHint("Check whether local CLI storage is accessible, then retry.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), tokenStorageLockTimeout)
	defer cancel()

	fileLock := flock.New(tokenStorageLockPath(appID, userOpenID))
	locked, err := fileLock.TryLockContext(ctx, tokenStorageLockRetryDelay)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errs.NewInternalError(errs.SubtypeStorage,
				"timed out waiting for token storage lock for user %q", userOpenID).
				WithRetryable().
				WithCause(err).
				WithHint("Retry the command.")
		}
		return errs.NewInternalError(errs.SubtypeFileIO,
			"failed to acquire token storage lock for user %q", userOpenID).
			WithCause(err).
			WithHint("Check whether local CLI storage is accessible, then retry.")
	}
	if !locked {
		return errs.NewInternalError(errs.SubtypeStorage,
			"timed out waiting for token storage lock for user %q", userOpenID).
			WithRetryable().
			WithCause(context.DeadlineExceeded).
			WithHint("Retry the command.")
	}
	defer func() {
		if unlockErr := fileLock.Unlock(); err == nil && unlockErr != nil {
			err = errs.NewInternalError(errs.SubtypeFileIO,
				"failed to release token storage lock").
				WithCause(unlockErr).
				WithHint("Retry the command. If this persists, check whether local CLI storage is accessible.")
		}
	}()
	return fn()
}
