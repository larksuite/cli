// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestVerifyCandidateVersionIgnoresStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "lark-cli")
	script := "#!/bin/sh\nprintf 'Fetching API metadata...\\n' >&2\nprintf 'lark-cli version 1.2.3\\n'\n"
	if err := vfs.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCandidateVersion(path, "1.2.3"); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateInstallPromotesStagedBinaryAndCleansBackup(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "lark-cli")
	backup := target + ".old"
	staged := filepath.Join(root, "staged")
	if err := vfs.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	finalize, err := (&Candidate{path: staged, target: target}).Install()
	if err != nil {
		t.Fatal(err)
	}
	got, err := vfs.ReadFile(target)
	if err != nil || string(got) != "new" {
		t.Fatalf("target = %q, %v", got, err)
	}
	finalize()
	if _, err := vfs.Stat(backup); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("backup remains after finalize: %v", err)
	}
}
