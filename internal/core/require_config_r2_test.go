// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubKCForTest matches keychain.KeychainAccess; never reached because
// LoadMultiAppConfig fails first on R2.
type stubKCForTest struct{}

func (stubKCForTest) Get(service, account string) (string, error) { return "", nil }
func (stubKCForTest) Set(service, account, value string) error    { return nil }
func (stubKCForTest) Remove(service, account string) error        { return nil }

// RequireConfigForProfileAndUser must surface R2 *ConfigError verbatim — the
// previous `if err != nil { return nil, NotConfiguredError() }` swallowed it,
// pushing AI agents toward `config init` which would overwrite newer-binary
// fields.
func TestRequireConfigForProfileAndUser_R2_PassesThroughConfigError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	// Write a config file that triggers R2 (SchemaVersion above the cap).
	future := []byte(`{"schemaVersion":99,"apps":[{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[]}]}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), future, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := RequireConfigForProfileAndUser(stubKCForTest{}, "", "")
	if err == nil {
		t.Fatal("expected R2 error, got nil")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T: %v", err, err)
	}
	// The R2 message and upgrade hint must reach the operator unchanged.
	if !strings.Contains(cfgErr.Message, "newer lark-cli") {
		t.Errorf("R2 message lost; got %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "upgrade lark-cli") {
		t.Errorf("R2 upgrade hint lost; got %q", cfgErr.Hint)
	}
	if cfgErr.Message == "not configured" {
		t.Errorf("R2 collapsed to 'not configured'; this is the regression")
	}
}

// Genuine missing-file path still returns NotConfiguredError().
func TestRequireConfigForProfileAndUser_FileMissing_StillReturnsNotConfigured(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceLocal)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	_, err := RequireConfigForProfileAndUser(stubKCForTest{}, "", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
	if cfgErr.Message != "not configured" {
		t.Errorf("missing-file message = %q, want \"not configured\"", cfgErr.Message)
	}
}
