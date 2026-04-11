// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"path/filepath"
	"testing"
)

// TestAuthLogDir_UsesValidatedLogDirEnv verifies that a valid absolute
// LARKSUITE_CLI_LOG_DIR is normalized and used as the auth log directory.
func TestAuthLogDir_UsesValidatedLogDirEnv(t *testing.T) {
	base := t.TempDir()
	var err error
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", base, err)
	}
	t.Setenv("LARKSUITE_CLI_LOG_DIR", filepath.Join(base, "logs", "..", "auth"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")

	got := authLogDir()
	want := filepath.Join(base, "auth")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}

// TestAuthLogDir_InvalidLogDirFallsBackToStateDir verifies that an invalid
// LARKSUITE_CLI_LOG_DIR falls back to LARKSUITE_CLI_STATE_DIR/logs.
func TestAuthLogDir_InvalidLogDirFallsBackToStateDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "relative-logs")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	stateDir := t.TempDir()
	var err error
	stateDir, err = filepath.EvalSymlinks(stateDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", stateDir, err)
	}
	t.Setenv("LARKSUITE_CLI_STATE_DIR", stateDir)

	got := authLogDir()
	want := filepath.Join(stateDir, "logs")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}
