// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

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
