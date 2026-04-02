// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func setupStrictModeTestConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:     "test-app",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.BrandFeishu,
			Users:     []core.AppUser{},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatal(err)
	}
}

func TestStrictMode_Show_Default(t *testing.T) {
	setupStrictModeTestConfig(t)

	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "off") {
		t.Errorf("expected 'off' in output, got: %s", stdout.String())
	}
}

func TestStrictMode_SetOn_Profile(t *testing.T) {
	setupStrictModeTestConfig(t)

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"on"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify config was saved
	multi, _ := core.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode == nil || !*app.StrictMode {
		t.Error("expected StrictMode=true on profile")
	}
}

func TestStrictMode_SetOn_Global(t *testing.T) {
	setupStrictModeTestConfig(t)

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"on", "--global"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	multi, _ := core.LoadMultiAppConfig()
	if !multi.StrictMode {
		t.Error("expected global StrictMode=true")
	}
}

func TestStrictMode_Reset(t *testing.T) {
	setupStrictModeTestConfig(t)

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})

	// First set it on
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"on"})
	cmd.Execute()

	// Then reset
	cmd = NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"--reset"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	multi, _ := core.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode != nil {
		t.Errorf("expected nil StrictMode after reset, got %v", *app.StrictMode)
	}
}

func TestStrictMode_Show_EnvOverride(t *testing.T) {
	setupStrictModeTestConfig(t)

	t.Setenv("LARKSUITE_CLI_STRICT_MODE", "true")

	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "on") {
		t.Errorf("expected 'on' in output, got: %s", out)
	}
	if !strings.Contains(out, "env") {
		t.Errorf("expected 'env' source in output, got: %s", out)
	}
}
