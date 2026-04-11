// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

type executableTestFS struct {
	vfs.OsFs
	exe string
}

func (f executableTestFS) Executable() (string, error) { return f.exe, nil }

func TestResolveExe(t *testing.T) {
	u := New()
	p, err := u.resolveExe()
	if err != nil {
		t.Fatalf("resolveExe() error: %v", err)
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got: %s", p)
	}
}

func TestPrepareSelfReplace_ReturnsNoError(t *testing.T) {
	u := New()
	restore, err := u.PrepareSelfReplace()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	restore()
}

func TestCleanupStaleFiles_NoPanic(t *testing.T) {
	u := New()
	u.CleanupStaleFiles()
}

func TestVerifyBinaryChecksVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "lark-cli")
	// Script prints version string matching real CLI format when --version is passed.
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"lark-cli version 2.0.0\"; exit 0; fi\nexit 12\n"
	if err := os.WriteFile(exe, []byte(script), 0755); err != nil {
		t.Fatalf("write test binary: %v", err)
	}

	// Mock vfs.Executable to return our test script, matching VerifyBinary's
	// primary lookup path. Also prepend to PATH for the LookPath fallback.
	origFS := vfs.DefaultFS
	vfs.DefaultFS = executableTestFS{OsFs: vfs.OsFs{}, exe: exe}
	t.Cleanup(func() { vfs.DefaultFS = origFS })

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	// Matching version → success.
	if err := New().VerifyBinary("2.0.0"); err != nil {
		t.Fatalf("VerifyBinary(matching) error = %v, want nil", err)
	}

	// Mismatched version → error.
	if err := New().VerifyBinary("3.0.0"); err == nil {
		t.Fatal("VerifyBinary(mismatched) expected error, got nil")
	}

	// Substring of actual version must not match (e.g. "0.0" is in "2.0.0").
	if err := New().VerifyBinary("0.0"); err == nil {
		t.Fatal("VerifyBinary(substring) expected error, got nil")
	}

	// Version that is a prefix of actual must not match (e.g. "2.0.0" in "12.0.0").
	// Binary reports "2.0.0", asking for "12.0.0" must fail.
	if err := New().VerifyBinary("12.0.0"); err == nil {
		t.Fatal("VerifyBinary(prefix-mismatch) expected error, got nil")
	}
}
