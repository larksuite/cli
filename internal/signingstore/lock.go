// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signingstore

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	storeLockTimeout    = 60 * time.Second
	storeLockRetryDelay = 500 * time.Millisecond
)

var processLocks sync.Map

// WithStoreLock serializes by the actual lock path. It is not reentrant;
// fn must not call another store method holding the same path.
func WithStoreLock(ctx context.Context, lockPath string, fn func() error) (err error) {
	lockContext := ctx
	cancel := func() {}
	if lockContext == nil {
		lockContext, cancel = context.WithTimeout(context.Background(), storeLockTimeout)
	} else if _, hasDeadline := lockContext.Deadline(); !hasDeadline {
		lockContext, cancel = context.WithTimeout(lockContext, storeLockTimeout)
	}
	defer cancel()

	lockDir, err := validate.SafeEnvDirPath(filepath.Dir(lockPath), "key store lock directory")
	if err != nil {
		return err
	}
	lockPath = filepath.Join(lockDir, filepath.Base(lockPath))
	candidate := make(chan struct{}, 1)
	candidate <- struct{}{}
	value, _ := processLocks.LoadOrStore(lockPath, candidate)
	processLock := value.(chan struct{})
	select {
	case <-lockContext.Done():
		return lockContext.Err()
	case <-processLock:
	}
	defer func() { processLock <- struct{}{} }()
	if err := lockContext.Err(); err != nil {
		return err
	}

	if err := vfs.MkdirAll(lockDir, 0700); err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "failed to prepare key storage lock").
			WithCause(err).
			WithHint("Check whether local CLI storage is accessible, then retry.")
	}
	fileLock := flock.New(lockPath)
	locked, err := fileLock.TryLockContext(lockContext, storeLockRetryDelay)
	if errors.Is(err, context.DeadlineExceeded) || (err == nil && !locked) {
		return errs.NewInternalError(errs.SubtypeStorage, "timed out waiting for key storage lock").
			WithRetryable().
			WithCause(context.DeadlineExceeded).
			WithHint("Retry the command.")
	}
	if err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "failed to acquire key storage lock").
			WithCause(err).
			WithHint("Check whether local CLI storage is accessible, then retry.")
	}
	defer func() {
		if unlockErr := fileLock.Unlock(); err == nil && unlockErr != nil {
			err = errs.NewInternalError(errs.SubtypeFileIO, "failed to release key storage lock").
				WithCause(unlockErr).
				WithHint("Retry the command. If this persists, check whether local CLI storage is accessible.")
		}
	}()
	return fn()
}
