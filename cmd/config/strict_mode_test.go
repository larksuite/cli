// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

type stubStrictModeProvider struct {
	name    string
	account *extcred.Account
}

func (p *stubStrictModeProvider) Name() string { return p.name }
func (p *stubStrictModeProvider) ResolveAccount(ctx context.Context) (*extcred.Account, error) {
	return p.account, nil
}
func (p *stubStrictModeProvider) ResolveToken(ctx context.Context, req extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

func setupStrictModeTestConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:     "test-app",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.BrandFeishu,
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
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "off") {
		t.Errorf("expected 'off' in output, got: %s", stdout.String())
	}
}

func TestStrictMode_SetBot_Profile(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := core.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode == nil || *app.StrictMode != core.StrictModeBot {
		t.Error("expected StrictMode=bot on profile")
	}
}

func TestStrictMode_SetUser_Profile(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"user"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := core.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode == nil || *app.StrictMode != core.StrictModeUser {
		t.Error("expected StrictMode=user on profile")
	}
}

func TestStrictMode_SetOff_Profile(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot"})
	cmd.Execute()
	cmd = NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"off"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := core.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode == nil || *app.StrictMode != core.StrictModeOff {
		t.Error("expected StrictMode=off on profile")
	}
}

func TestStrictMode_SetBot_Global(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot", "--global"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := core.LoadMultiAppConfig()
	if multi.StrictMode != core.StrictModeBot {
		t.Error("expected global StrictMode=bot")
	}
}

func TestStrictMode_Reset(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot"})
	cmd.Execute()
	cmd = NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"--reset"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	multi, _ := core.LoadMultiAppConfig()
	app := multi.CurrentAppConfig("")
	if app.StrictMode != nil {
		t.Errorf("expected nil StrictMode after reset, got %v", *app.StrictMode)
	}
}

func TestStrictMode_InvalidValue(t *testing.T) {
	setupStrictModeTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"on"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid value 'on'")
	}
}

func TestStrictMode_Show_PrefersExternalCredentialSourceEvenWhenValueMatchesConfig(t *testing.T) {
	setupStrictModeTestConfig(t)

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	mode := core.StrictModeBot
	multi.Apps[0].StrictMode = &mode
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{&stubStrictModeProvider{
			name: "env",
			account: &extcred.Account{
				AppID:               "env-app",
				AppSecret:           "env-secret",
				Brand:               string(core.BrandFeishu),
				SupportedIdentities: extcred.SupportsBot,
			},
		}},
		nil,
		nil,
		nil,
	)

	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	want := `strict-mode: bot (source: credential provider "env")`
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("output = %q, want substring %q", stdout.String(), want)
	}
}
