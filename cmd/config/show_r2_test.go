// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// Regression: `config show` previously wrapped a non-os.ErrNotExist
// load error in errs.NewConfigError(SubtypeInvalidConfig, "failed to
// load config: %v", err).WithCause(err). The dispatcher's
// PromoteConfigError step is gated on isOuterTypedError — when the
// outer envelope is already a typed *errs.ConfigError it short-
// circuits and uses the producer's coarser shape, hiding the R2
// upgrade hint behind a generic "failed to load config" message.
//
// PassThroughOrNotConfigured returns the raw *core.ConfigError so
// the dispatcher can promote it with the upgrade Hint preserved.
func TestConfigShowRun_R2ForwardIncompat_PassesUpgradeHint(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	t.Setenv("HOME", t.TempDir())

	future := []byte(`{"schemaVersion":99,"apps":[{"appId":"cli_x","appSecret":"s","brand":"feishu","users":[]}]}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), future, 0600); err != nil {
		t.Fatalf("seed config.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err == nil {
		t.Fatal("config show must surface R2 error from a future schema, got nil")
	}
	// Outer-type assertion: the dispatcher's isOuterTypedError gate
	// short-circuits PromoteConfigError when the outer is already a
	// typed *errs.* envelope. The producer must hand back the raw
	// *core.ConfigError so promotion routes the R2 hint.
	if _, ok := err.(*core.ConfigError); !ok {
		t.Fatalf("expected outer *core.ConfigError so dispatcher routes R2 hint; got %T: %v", err, err)
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

// Regression for A9: --profile=ghost on a populated config must NOT
// route to SubtypeNotConfigured. The config IS configured — the
// operator just typed a name that doesn't exist. SubtypeNotConfigured
// would steer AI agents to `config init` and clobber the working
// profiles. SubtypeInvalidArgument is the correct routing axis.
func TestConfigShowRun_ExplicitProfileNotFound_SubtypeIsInvalidArgument(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	cfg := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name: "alpha", AppId: "cli_a", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
		}},
		CurrentApp: "alpha",
	}
	if err := core.SaveMultiAppConfig(cfg); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Invocation = cmdutil.InvocationContext{Profile: "ghost"}
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err == nil {
		t.Fatal("expected error for ghost --profile, got nil")
	}
	var typed *errs.ConfigError
	if !errors.As(err, &typed) {
		t.Fatalf("expected *errs.ConfigError; got %T %v", err, err)
	}
	if typed.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype: got %q, want %q (SubtypeInvalidArgument; "+
			"SubtypeNotConfigured would invite config init)",
			typed.Subtype, errs.SubtypeInvalidArgument)
	}
	if !strings.Contains(typed.Message+typed.Hint, "ghost") {
		t.Errorf("error must name the bad profile; got msg=%q hint=%q", typed.Message, typed.Hint)
	}
}

// Stored CurrentApp dangles (config exists but no resolvable active):
// SubtypeInvalidConfig, not SubtypeNotConfigured. Re-binding /
// `profile use <name>` is the right next step, NOT re-init.
func TestConfigShowRun_DanglingCurrentApp_SubtypeIsInvalidConfig(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	cfg := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name: "alpha", AppId: "cli_a", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
		}},
		CurrentApp: "ghost", // dangles — no Apps[i].Name == "ghost"
	}
	if err := core.SaveMultiAppConfig(cfg); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err == nil {
		t.Fatal("expected error for dangling CurrentApp, got nil")
	}
	var typed *errs.ConfigError
	if !errors.As(err, &typed) {
		t.Fatalf("expected *errs.ConfigError; got %T %v", err, err)
	}
	if typed.Subtype != errs.SubtypeInvalidConfig {
		t.Errorf("subtype: got %q, want %q (SubtypeInvalidConfig; "+
			"SubtypeNotConfigured would invite config init)",
			typed.Subtype, errs.SubtypeInvalidConfig)
	}
}
