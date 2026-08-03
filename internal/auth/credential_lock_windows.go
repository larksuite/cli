// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/larksuite/cli/errs"
	"golang.org/x/sys/windows"
)

func withCredentialLock(appID, userOpenID string, operation func() error) error {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to resolve Windows user for credential lock").WithCause(err)
	}
	digest := sha256.Sum256([]byte(tokenUser.User.Sid.String() + "\x00" + appID + "\x00" + userOpenID))
	name, err := windows.UTF16PtrFromString(`Global\LarkCliCredential_` + hex.EncodeToString(digest[:16]))
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to create Windows credential lock name").WithCause(err)
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to create Windows credential lock").WithCause(err)
	}
	defer windows.CloseHandle(handle)

	result, err := windows.WaitForSingleObject(handle, 30_000)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to acquire Windows credential lock").WithCause(err)
	}
	if result != windows.WAIT_OBJECT_0 && result != windows.WAIT_ABANDONED {
		return errs.NewInternalError(errs.SubtypeStorage, "timed out waiting for Windows credential lock")
	}
	defer windows.ReleaseMutex(handle)
	return operation()
}
