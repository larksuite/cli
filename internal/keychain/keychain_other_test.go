// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build linux

package keychain

import (
	"path/filepath"
	"testing"
)

// TestStorageDir_UsesValidatedDataDirEnv verifies that a valid absolute
// LARKSUITE_CLI_DATA_DIR is normalized and still preserves service isolation.
func TestStorageDir_UsesValidatedDataDirEnv(t *testing.T) {
	base := t.TempDir()
	base, _ = filepath.EvalSymlinks(base)
	t.Setenv("LARKSUITE_CLI_DATA_DIR", filepath.Join(base, "data", "..", "store"))

	got := StorageDir("svc")
	want := filepath.Join(base, "store", "svc")
	if got != want {
		t.Fatalf("StorageDir() = %q, want %q", got, want)
	}
}

// TestStorageDir_InvalidDataDirFallsBackToDefault verifies that an invalid
// LARKSUITE_CLI_DATA_DIR falls back to the default per-service storage path.
func TestStorageDir_InvalidDataDirFallsBackToDefault(t *testing.T) {
	home := t.TempDir()
	home, _ = filepath.EvalSymlinks(home)
	t.Setenv("LARKSUITE_CLI_DATA_DIR", "relative-data")
	t.Setenv("HOME", home)

	got := StorageDir("svc")
	want := filepath.Join(home, ".local", "share", "svc")
	if got != want {
		t.Fatalf("StorageDir() = %q, want %q", got, want)
	}
}
