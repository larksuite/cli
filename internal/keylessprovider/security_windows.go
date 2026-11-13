// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package keylessprovider

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/larksuite/cli/internal/vfs"
	"golang.org/x/sys/windows"
)

func validateProviderObject(path string, wantDir bool) error {
	info, err := vfs.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect provider object %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("provider object must not be a symlink: %s", path)
	}
	if wantDir != info.IsDir() || (!wantDir && !info.Mode().IsRegular()) {
		return fmt.Errorf("provider object has unexpected type: %s", path)
	}

	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encode provider object path: %w", err)
	}
	attrs, err := windows.GetFileAttributes(path16)
	if err != nil {
		return fmt.Errorf("inspect provider object attributes: %w", err)
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("provider object must not be a reparse point: %s", path)
	}

	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("inspect provider object security descriptor: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return fmt.Errorf("inspect provider object owner: %w", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("inspect current Windows user: %w", err)
	}
	if !owner.Equals(user.User.Sid) {
		return fmt.Errorf("provider object is not owned by the current user: %s", path)
	}
	if err := validateWindowsDACL(sd, user.User.Sid); err != nil {
		return fmt.Errorf("provider object has unsafe permissions: %s: %w", path, err)
	}
	return nil
}

func validateInspectExecutable(path string) error {
	info, err := vfs.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect OpenClaw executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("OpenClaw executable must be a regular non-symlink file")
	}
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(path16)
	if err != nil || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("OpenClaw executable must not be a reparse point")
	}
	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return fmt.Errorf("inspect OpenClaw executable owner: %w", err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if !owner.Equals(user.User.Sid) &&
		!owner.IsWellKnown(windows.WinLocalSystemSid) &&
		!owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
		return fmt.Errorf("OpenClaw executable has an untrusted owner")
	}
	if err := validateWindowsDACL(sd, user.User.Sid); err != nil {
		return fmt.Errorf("OpenClaw executable has unsafe permissions: %w", err)
	}
	return nil
}

func validateWindowsDACL(sd *windows.SECURITY_DESCRIPTOR, currentUser *windows.SID) error {
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("missing or unreadable DACL")
	}
	const (
		fileDeleteChild windows.ACCESS_MASK = 0x00000040
		writeMask                           = windows.GENERIC_ALL | windows.GENERIC_WRITE |
			windows.WRITE_DAC | windows.WRITE_OWNER | windows.DELETE |
			windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA |
			windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES | fileDeleteChild
	)
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			return fmt.Errorf("read ACL entry %d: %w", i, err)
		}
		if ace == nil || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("unsupported allow ACL entry type %d", ace.Header.AceType)
		}
		if ace.Mask&writeMask == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.IsValid() {
			return fmt.Errorf("ACL entry %d has invalid SID", i)
		}
		if sid.Equals(currentUser) ||
			sid.IsWellKnown(windows.WinLocalSystemSid) ||
			sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) ||
			sid.IsWellKnown(windows.WinCreatorOwnerSid) ||
			sid.IsWellKnown(windows.WinCreatorOwnerRightsSid) {
			continue
		}
		return fmt.Errorf("write access is granted to SID %s", sid.String())
	}
	return nil
}
