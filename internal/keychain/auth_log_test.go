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
	base, _ = filepath.EvalSymlinks(base)
	t.Setenv("LARKSUITE_CLI_LOG_DIR", filepath.Join(base, "logs", "..", "auth"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")

	got := authLogDir()
	want := filepath.Join(base, "auth")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}

// TestAuthLogDir_InvalidLogDirFallsBackToConfigDir verifies that an invalid
// LARKSUITE_CLI_LOG_DIR falls back to LARKSUITE_CLI_CONFIG_DIR/logs.
func TestAuthLogDir_InvalidLogDirFallsBackToConfigDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "relative-logs")
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	got := authLogDir()
	want := filepath.Join(configDir, "logs")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}
