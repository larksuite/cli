// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/brand"
	"github.com/larksuite/cli/internal/cmdutil"
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/identity"
	"github.com/larksuite/cli/internal/secret"
)

func setupStrictModeTestConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	multi := &configpkg.MultiAppConfig{
		Apps: []configpkg.AppConfig{{
			AppId:     "test-app",
			AppSecret: secret.PlainSecret("secret"),
			Brand:     brand.Feishu,
		}},
	}
	if err := configpkg.SaveMultiAppConfig(multi); err != nil {
		t.Fatal(err)
	}
}

func TestStrictMode_Show_Default(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, stdout, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "off") {
		t.Errorf("expected 'off' in output, got: %s", stdout.String())
	}
}

func TestStrictMode_SetBot_Profile(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := configpkg.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode == nil || *app.StrictMode != identity.StrictModeBot {
		t.Error("expected StrictMode=bot on profile")
	}
}

func TestStrictMode_SetUser_Profile(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"user"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := configpkg.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode == nil || *app.StrictMode != identity.StrictModeUser {
		t.Error("expected StrictMode=user on profile")
	}
}

func TestStrictMode_SetOff_Profile(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot"})
	cmd.Execute()
	cmd = NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"off"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := configpkg.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode == nil || *app.StrictMode != identity.StrictModeOff {
		t.Error("expected StrictMode=off on profile")
	}
}

func TestStrictMode_SetBot_Global(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot", "--global"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := configpkg.LoadMultiAppConfig()
	if multi.StrictMode != identity.StrictModeBot {
		t.Error("expected global StrictMode=bot")
	}
}

func TestStrictMode_SetGlobal_DoesNotRequireActiveProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	multi := &configpkg.MultiAppConfig{
		CurrentApp: "missing-profile",
		Apps: []configpkg.AppConfig{{
			Name:      "default",
			AppId:     "test-app",
			AppSecret: secret.PlainSecret("secret"),
			Brand:     brand.Feishu,
		}},
	}
	if err := configpkg.SaveMultiAppConfig(multi); err != nil {
		t.Fatal(err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot", "--global"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	saved, err := configpkg.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.StrictMode != identity.StrictModeBot {
		t.Fatalf("StrictMode = %q, want %q", saved.StrictMode, identity.StrictModeBot)
	}
}

func TestStrictMode_Reset(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot"})
	cmd.Execute()
	cmd = NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"--reset"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := configpkg.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode != nil {
		t.Errorf("expected nil StrictMode after reset, got %v", *app.StrictMode)
	}
}

func TestStrictMode_InvalidValue(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &configpkg.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"on"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid value 'on'")
	}
}
