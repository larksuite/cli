// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package selfupdate

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestCandidateInstallRestoresPartialWindowsReplacement(t *testing.T) {
	target := t.TempDir() + `\lark-cli.exe`
	candidate := target + ".new"
	if err := vfs.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(candidate, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := replaceFilePath
	replaceFilePath = func(targetPath, _ string, backupPath string) error {
		if err := vfs.Rename(targetPath, backupPath); err != nil {
			t.Fatal(err)
		}
		return errors.New("simulated partial replacement")
	}
	t.Cleanup(func() { replaceFilePath = original })

	prepared := &Candidate{path: candidate, target: target}
	if _, err := prepared.Install(); err == nil {
		t.Fatal("partial replacement unexpectedly succeeded")
	}
	got, err := vfs.ReadFile(target)
	if err != nil || string(got) != "old" {
		t.Fatalf("restored target = %q, %v", got, err)
	}
}
