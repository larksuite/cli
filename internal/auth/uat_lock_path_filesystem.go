// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin || linux

package auth

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/vfs"
)

func refreshLockDir() string {
	return filepath.Join(keychain.StorageDir(keychain.LarkCliService), "locks")
}

func refreshLockPath(appID, userOpenID string) string {
	return filepath.Join(refreshLockDir(), fmt.Sprintf("refresh_%s_%s.lock", sanitizeID(appID), sanitizeID(userOpenID)))
}

func withCredentialLock(appID, userOpenID string, operation func() error) error {
	lockFile := refreshLockPath(appID, userOpenID)
	if err := vfs.MkdirAll(filepath.Dir(lockFile), 0700); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to create credential lock directory").WithCause(err)
	}

	fileLock := flock.New(lockFile)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	locked, err := fileLock.TryLockContext(ctx, 500*time.Millisecond)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to acquire credential lock").WithCause(err)
	}
	if !locked {
		return errs.NewInternalError(errs.SubtypeStorage, "timed out waiting for credential lock")
	}
	defer fileLock.Unlock()
	return operation()
}
