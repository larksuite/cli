// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended && windows

package externalcredential

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/larksuite/cli/internal/vfs"
	"golang.org/x/sys/windows"
)

func validateAdminControlledPath(target string, executable bool) error {
	if !filepath.IsAbs(target) {
		return fmt.Errorf("%s must be an absolute path", target)
	}
	info, err := vfs.Lstat(target)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular non-symlink file", target)
	}
	if executable && !strings.EqualFold(filepath.Ext(target), ".exe") {
		return fmt.Errorf("%s must be a native .exe file", target)
	}

	for current, file := filepath.Clean(target), true; ; current, file = filepath.Dir(current), false {
		if err := validateWindowsACL(current, file); err != nil {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return nil
}

func validateWindowsACL(path string, file bool) error {
	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("cannot inspect ACL for %s: %w", path, err)
	}

	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("cannot resolve LocalSystem SID: %w", err)
	}
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("cannot resolve Administrators SID: %w", err)
	}
	trustedInstallerSID, err := windows.StringToSid("S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464")
	if err != nil {
		return fmt.Errorf("cannot resolve TrustedInstaller SID: %w", err)
	}
	trustedSIDs := []*windows.SID{systemSID, adminSID, trustedInstallerSID}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("cannot inspect owner for %s: %w", path, err)
	}
	if !sidIn(owner, trustedSIDs) {
		return fmt.Errorf("%s must be owned by LocalSystem, Administrators, or TrustedInstaller", path)
	}

	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("cannot inspect DACL for %s: %w", path, err)
	}
	if dacl == nil {
		return fmt.Errorf("%s has a permissive empty DACL", path)
	}
	dangerousMask := dangerousWindowsAccessMask(file)
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil {
			return fmt.Errorf("cannot inspect DACL entry for %s: %w", path, err)
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 ||
			ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE ||
			ace.Mask&dangerousMask == 0 {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("%s uses an unsupported writable ACL entry", path)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sidIn(sid, trustedSIDs) {
			return fmt.Errorf("%s grants write access to a non-administrator principal", path)
		}
	}
	return nil
}

func dangerousWindowsAccessMask(file bool) windows.ACCESS_MASK {
	mask := windows.ACCESS_MASK(
		windows.DELETE |
			windows.WRITE_DAC |
			windows.WRITE_OWNER |
			windows.GENERIC_ALL |
			windows.GENERIC_WRITE,
	)
	if file {
		// Do not use FILE_GENERIC_WRITE here: it is a composite mask that also
		// contains READ_CONTROL and SYNCHRONIZE, which are legitimately present
		// in read-only ACEs on Windows system files.
		mask |= windows.FILE_WRITE_DATA |
			windows.FILE_APPEND_DATA |
			windows.FILE_WRITE_EA |
			windows.FILE_WRITE_ATTRIBUTES
	} else {
		// A parent only needs to prevent replacement of the protected child.
		// Creating unrelated files beside it does not weaken that boundary.
		const fileDeleteChild windows.ACCESS_MASK = 0x00000040
		mask |= fileDeleteChild
	}
	return mask
}

func sidIn(candidate *windows.SID, allowed []*windows.SID) bool {
	for _, sid := range allowed {
		if windows.EqualSid(candidate, sid) {
			return true
		}
	}
	return false
}
