// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package lockfile

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32    = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = modkernel32.NewProc("LockFileEx")
	procUnlockFile = modkernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
)

func tryLockFile(f *os.File) error {
	var ol syscall.Overlapped
	handle := syscall.Handle(f.Fd())
	r1, _, err := procLockFileEx.Call(
		uintptr(handle),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		1, 0,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		return fmt.Errorf("%w (lock: %s, syscall: %v)", ErrHeld, f.Name(), err)
	}
	return nil
}

func unlockFile(f *os.File) error {
	var ol syscall.Overlapped
	handle := syscall.Handle(f.Fd())
	r1, _, err := procUnlockFile.Call(
		uintptr(handle),
		0,
		1, 0,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}
