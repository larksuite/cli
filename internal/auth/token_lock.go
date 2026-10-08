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
	tokenStorageLockTimeout    = 60 * time.Second
	tokenStorageLockRetryDelay = 500 * time.Millisecond
	tatIssuanceLockTimeout     = 60 * time.Second
)

var tokenStorageProcessLocks sync.Map
var tatIssuanceProcessLocks sync.Map

var safeIDChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// sanitizeID replaces characters that are unsafe in lock filenames.
func sanitizeID(id string) string {
	return safeIDChars.ReplaceAllString(id, "_")
}

func tokenStorageLockDir() string {
	return filepath.Join(core.GetConfigDir(), "locks")
}

func tokenStorageLockPath(appID, userOpenID string) string {
	return filepath.Join(tokenStorageLockDir(), fmt.Sprintf("refresh_%s_%s.lock",
		sanitizeID(appID), sanitizeID(userOpenID)))
}

func tokenStorageProcessLock(appID, userOpenID string) chan struct{} {
	key := accountKey(appID, userOpenID)
	candidate := make(chan struct{}, 1)
	candidate <- struct{}{}
	lock, _ := tokenStorageProcessLocks.LoadOrStore(key, candidate)
	return lock.(chan struct{})
}

// WithTATIssuanceLock runs fn while holding the process-local TAT issuance lock
// for one app.
func WithTATIssuanceLock(ctx context.Context, appID string, fn func() error) error {
	lockContext := ctx
	cancel := func() {}
	if lockContext == nil {
		lockContext, cancel = context.WithTimeout(context.Background(), tatIssuanceLockTimeout)
	} else if _, hasDeadline := lockContext.Deadline(); !hasDeadline {
		lockContext, cancel = context.WithTimeout(lockContext, tatIssuanceLockTimeout)
	}
	defer cancel()

	candidate := make(chan struct{}, 1)
	candidate <- struct{}{}
	value, _ := tatIssuanceProcessLocks.LoadOrStore(appID, candidate)
	processLock := value.(chan struct{})
	select {
	case <-lockContext.Done():
		return lockContext.Err()
	case <-processLock:
	}
	defer func() { processLock <- struct{}{} }()
	return fn()
}

// withTokenStorageLock runs fn while holding both the process-local and
// cross-process locks for one account. The lock is not reentrant: fn must not
// call SetStoredToken, RemoveStoredToken, refreshWithLock, or another mutation
// path that reacquires the lock for the same account, because doing so will
// deadlock. Token storage mutations inside fn must use helpers that require the
// caller to hold the lock, such as writeStoredToken, deleteStoredToken,
// compareAndSwapStoredToken, or compareAndDeleteStoredToken.
func withTokenStorageLock(appID, userOpenID string, fn func() error) (err error) {
	return withTokenStorageLockContext(context.Background(), appID, userOpenID, fn)
}

func withTokenStorageLockContext(ctx context.Context, appID, userOpenID string, fn func() error) (err error) {
	lockContext := ctx
	cancel := func() {}
	if lockContext == nil {
		lockContext, cancel = context.WithTimeout(context.Background(), tokenStorageLockTimeout)
	} else if _, hasDeadline := lockContext.Deadline(); !hasDeadline {
		lockContext, cancel = context.WithTimeout(lockContext, tokenStorageLockTimeout)
	}
	defer cancel()

	processLock := tokenStorageProcessLock(appID, userOpenID)
	select {
	case <-lockContext.Done():
		return lockContext.Err()
	case <-processLock:
	}
	defer func() { processLock <- struct{}{} }()

	lockDir := tokenStorageLockDir()
	if err := vfs.MkdirAll(lockDir, 0700); err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO,
			"failed to prepare token storage lock").
			WithCause(err).
			WithHint("Check whether local CLI storage is accessible, then retry.")
	}

	fileLock := flock.New(tokenStorageLockPath(appID, userOpenID))
	locked, err := fileLock.TryLockContext(lockContext, tokenStorageLockRetryDelay)
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
