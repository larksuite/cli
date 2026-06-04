// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// Regression: a config.json written by a newer lark-cli (schemaVersion >
// CurrentSchemaVersion) must surface its R2 *core.ConfigError + upgrade
// Hint through `auth list` and `auth users list`. Pre-fix, both paths
// did `multi, _ := core.LoadMultiAppConfig()` and silently dropped the
// error — `auth list` continued on with multi==nil and printed the
// generic "not configured" hint, steering operators (and AI agents)
// toward `config init --new`, which would overwrite the fields the
// newer binary populated.
//
// Mirrors the contract pinned at
// internal/credential/default_provider_r2_test.go (ResolveAccount) and
// internal/errcompat/promote_r2_test.go (dispatcher promotion).
func TestAuthListRun_R2ForwardIncompat_PassesUpgradeHint(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	t.Setenv("HOME", t.TempDir())

	future := []byte(`{"schemaVersion":99,"apps":[{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[]}]}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), future, 0600); err != nil {
		t.Fatalf("seed config.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := authListRun(&ListOptions{Factory: f})
	if err == nil {
		t.Fatal("auth list must surface R2 error from a future schema, got nil — would steer operator to config init --new")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *core.ConfigError preserved by PassThroughOrNotConfigured, got %T: %v", err, err)
	}
	if !strings.Contains(cfgErr.Message, "newer lark-cli") {
		t.Errorf("R2 message lost; got %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "upgrade lark-cli") {
		t.Errorf("R2 upgrade hint lost; got %q", cfgErr.Hint)
	}
}

// Same contract for `auth users list`. Same root cause pre-fix.
func TestAuthUsersListRun_R2ForwardIncompat_PassesUpgradeHint(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	t.Setenv("HOME", t.TempDir())

	future := []byte(`{"schemaVersion":99,"apps":[{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[]}]}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), future, 0600); err != nil {
		t.Fatalf("seed config.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := authUsersListRun(&UsersListOptions{Factory: f})
	if err == nil {
		t.Fatal("auth users list must surface R2 error from a future schema, got nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *core.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(cfgErr.Message, "newer lark-cli") {
		t.Errorf("R2 message lost; got %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "upgrade lark-cli") {
		t.Errorf("R2 upgrade hint lost; got %q", cfgErr.Hint)
	}
}
