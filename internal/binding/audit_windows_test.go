// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package binding

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssertSecurePath_WindowsIgnoresSyntheticUnixPermissionBits(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secrets-getter.cmd")
	if err := os.WriteFile(p, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatalf("write temp command: %v", err)
	}

	got, err := AssertSecurePath(AuditParams{
		TargetPath:            p,
		Label:                 "exec provider command",
		AllowInsecurePath:     false,
		AllowReadableByOthers: true,
	})
	if err != nil {
		t.Fatalf("unexpected error for Windows synthetic mode bits: %v", err)
	}
	if got != p {
		t.Errorf("got %q, want %q", got, p)
	}
}
