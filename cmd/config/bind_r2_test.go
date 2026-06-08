// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// TestConfigBindRun_R2_RefusesForwardIncompatConfig is the read-side gate.
//
// Pre-fix `reconcileExistingBinding` read the existing config via raw
// vfs.ReadFile + json.Unmarshal, bypassing core.LoadMultiAppConfig's
// SchemaVersion gate. A workspace whose config.json was written by a newer
// lark-cli (schemaVersion > CurrentSchemaVersion) would be silently
// overwritten on rebind, dropping any newer-only fields the future binary
// had populated.
//
// The fix probes SchemaVersion before either the TUI conflict prompt or the
// flag-mode silent overwrite path, and returns a *core.ConfigError that
// reaches the user verbatim (configBindRun returns it without wrapping —
// see bind.go:158).
func TestConfigBindRun_R2_RefusesForwardIncompatConfig(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	// Pre-create hermes workspace config tagged with a future schemaVersion.
	// Only schemaVersion matters here — the rest of the body just needs to
	// be parseable JSON for the json.Unmarshal probe to succeed.
	hermesDir := filepath.Join(configDir, "hermes")
	if err := os.MkdirAll(hermesDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	hermesConfigPath := filepath.Join(hermesDir, "config.json")
	if err := os.WriteFile(hermesConfigPath,
		[]byte(`{"schemaVersion":2,"apps":[{"appId":"future_app"}]}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Stage a Hermes source so resolveAccount would otherwise succeed —
	// proving the refusal fires at reconcile, not later.
	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"),
		[]byte("FEISHU_APP_ID=cli_new_app\nFEISHU_APP_SECRET=new_secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	if err == nil {
		t.Fatal("expected refusal for schemaVersion > CurrentSchemaVersion, got nil")
	}

	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *core.ConfigError", err)
	}
	if cfgErr.Code != 3 {
		t.Errorf("Code = %d, want 3 (config exit code)", cfgErr.Code)
	}
	if cfgErr.Type != "config" {
		t.Errorf("Type = %q, want %q", cfgErr.Type, "config")
	}
	if !strings.Contains(cfgErr.Message, "schemaVersion 2") {
		t.Errorf("message missing 'schemaVersion 2': %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Message, "refusing to overwrite") {
		t.Errorf("message missing 'refusing to overwrite': %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "upgrade lark-cli") {
		t.Errorf("hint missing 'upgrade lark-cli': %q", cfgErr.Hint)
	}

	// Critical: the file must be untouched. Pre-fix it would have been
	// silently overwritten with the new flag-mode binding.
	got, err := os.ReadFile(hermesConfigPath)
	if err != nil {
		t.Fatalf("read after refusal: %v", err)
	}
	if !strings.Contains(string(got), `"schemaVersion":2`) ||
		!strings.Contains(string(got), `"future_app"`) {
		t.Errorf("config was overwritten despite refusal:\n%s", got)
	}
}

// TestConfigBindRun_R2_StampsCurrentSchemaVersion is the write-side gate.
//
// Pre-fix `commitBinding` wrote the new config via inline json.MarshalIndent
// + validate.AtomicWrite, skipping core.SaveMultiAppConfig's
// SchemaVersion-stamping. A successful bind would leave the file at
// SchemaVersion=0 (legacy), which defeats the read-side gate's whole
// purpose: as soon as any forward-incompat field is introduced in a future
// version, the file looks legacy and the gate silently lets it through.
//
// The fix migrates the write to core.SaveMultiAppConfig, which stamps
// SchemaVersion = CurrentSchemaVersion. Round-tripping via
// core.LoadMultiAppConfig confirms the stamp is durably persisted.
func TestConfigBindRun_R2_StampsCurrentSchemaVersion(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"),
		[]byte("FEISHU_APP_ID=cli_stamp_test\nFEISHU_APP_SECRET=stamp_secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f, Source: "hermes"}); err != nil {
		t.Fatalf("configBindRun: %v", err)
	}

	// SetCurrentWorkspace was invoked during configBindRun; LoadMultiAppConfig
	// reads from the workspace path the bind targeted.
	loaded, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig after bind: %v", err)
	}
	if loaded.SchemaVersion != core.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d (CurrentSchemaVersion)",
			loaded.SchemaVersion, core.CurrentSchemaVersion)
	}
	if len(loaded.Apps) != 1 || loaded.Apps[0].AppId != "cli_stamp_test" {
		t.Errorf("apps = %+v, want single cli_stamp_test", loaded.Apps)
	}
}
