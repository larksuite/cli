// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/vfs"
)

const authConfigLockWait = 30 * time.Second

// withAuthConfigLock serializes read-modify-write operations on config.json.
// Token storage keeps its own per-user lock; this lock owns only selection and
// membership changes in the auth/config surface.
func withAuthConfigLock(fn func() error) error {
	dir := filepath.Join(core.GetConfigDir(), "locks")
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "failed to prepare auth config lock: %v", err).WithCause(err)
	}

	lock := lockfile.New(filepath.Join(dir, "auth_config.lock"))
	deadline := time.Now().Add(authConfigLockWait)
	for {
		err := lock.TryLock()
		if err == nil {
			runErr := fn()
			unlockErr := lock.Unlock()
			if runErr != nil {
				return runErr
			}
			if unlockErr != nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "failed to release auth config lock: %v", unlockErr).WithCause(unlockErr)
			}
			return nil
		}
		if !errors.Is(err, lockfile.ErrHeld) {
			return errs.NewInternalError(errs.SubtypeFileIO, "failed to acquire auth config lock: %v", err).WithCause(err)
		}
		if time.Now().After(deadline) {
			return errs.NewInternalError(errs.SubtypeStorage, "timed out waiting for auth config lock").
				WithRetryable().
				WithHint("Retry the command.")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
