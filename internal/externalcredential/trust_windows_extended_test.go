// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended && windows

package externalcredential

import (
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
	"golang.org/x/sys/windows"
)

// TestNativeAdminControlledPath exercises the production Windows owner/DACL
// inspection on a native runner. Cross-compilation alone cannot validate these
// security descriptors.
func TestNativeAdminControlledPath(t *testing.T) {
	windowsDir, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		t.Fatal(err)
	}
	systemExecutable := filepath.Join(windowsDir, "System32", "cmd.exe")
	if err := validateAdminControlledPath(systemExecutable, true); err != nil {
		t.Fatalf("trusted system executable %s was rejected: %v", systemExecutable, err)
	}

	untrusted := filepath.Join(t.TempDir(), "credential-helper.exe")
	if err := vfs.WriteFile(untrusted, []byte("native-placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateAdminControlledPath(untrusted, true); err == nil {
		t.Fatalf("caller-controlled executable %s was accepted", untrusted)
	}
}

func TestWindowsWriteMaskDoesNotOverlapReadOnlyRights(t *testing.T) {
	dangerousMask := dangerousWindowsAccessMask(true)
	readOnlyMask := windows.ACCESS_MASK(
		windows.FILE_GENERIC_READ |
			windows.FILE_GENERIC_EXECUTE,
	)
	if overlap := dangerousMask & readOnlyMask; overlap != 0 {
		t.Fatalf("write mask overlaps read-only rights: %#x", overlap)
	}
	for _, writeRight := range []windows.ACCESS_MASK{
		windows.GENERIC_WRITE,
		windows.FILE_WRITE_DATA,
		windows.FILE_APPEND_DATA,
		windows.FILE_WRITE_EA,
		windows.FILE_WRITE_ATTRIBUTES,
	} {
		if dangerousMask&writeRight == 0 {
			t.Fatalf("write mask omits mutation right %#x", writeRight)
		}
	}
}
