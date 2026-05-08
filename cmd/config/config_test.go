// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
)

type noopConfigKeychain struct{}

func (n *noopConfigKeychain) Get(service, account string) (string, error) { return "", nil }
func (n *noopConfigKeychain) Set(service, account, value string) error    { return nil }
func (n *noopConfigKeychain) Remove(service, account string) error        { return nil }

type recordingConfigKeychain struct {
	removed []string
}

func (r *recordingConfigKeychain) Get(service, account string) (string, error) { return "", nil }
func (r *recordingConfigKeychain) Set(service, account, value string) error    { return nil }
func (r *recordingConfigKeychain) Remove(service, account string) error {
	r.removed = append(r.removed, service+":"+account)
	return nil
}

func TestConfigInitCmd_FlagParsing(t *testing.T) {
	clearAgentEnv(t) // assumes local workspace; guard refuses init in agent contexts
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("secret123\n")

	var gotOpts *ConfigInitOptions
	cmd := NewCmdConfigInit(f, func(opts *ConfigInitOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--app-id", "cli_test", "--app-secret-stdin", "--brand", "lark"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.AppID != "cli_test" {
		t.Errorf("expected AppID cli_test, got %s", gotOpts.AppID)
	}
	if !gotOpts.AppSecretStdin {
		t.Error("expected AppSecretStdin=true")
	}
	if gotOpts.Brand != "lark" {
		t.Errorf("expected Brand lark, got %s", gotOpts.Brand)
	}
}

func TestConfigShowCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *ConfigShowOptions
	cmd := NewCmdConfigShow(f, func(opts *ConfigShowOptions) error {
		gotOpts = opts
		return nil
	})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts == nil {
		t.Error("expected opts to be set")
	}
}

func TestConfigShowRun_NotConfiguredReturnsStructuredError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err == nil {
		t.Fatal("expected error")
	}

	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *core.ConfigError", err)
	}
	if cfgErr.Code != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d", cfgErr.Code, output.ExitValidation)
	}
	if cfgErr.Type != "config" || cfgErr.Message != "not configured" {
		t.Fatalf("detail = %+v, want config/not configured", cfgErr)
	}
}

func TestConfigShowRun_NoActiveProfileReturnsStructuredError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	multi := &core.MultiAppConfig{
		CurrentApp: "missing",
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "app-default",
			AppSecret: core.PlainSecret("secret-default"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *output.ExitError", err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "config" || exitErr.Detail.Message != "no active profile" {
		t.Fatalf("detail = %#v, want config/no active profile", exitErr.Detail)
	}
}

func TestConfigInitCmd_LangFlag(t *testing.T) {
	clearAgentEnv(t) // assumes local workspace; guard refuses init in agent contexts
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *ConfigInitOptions
	cmd := NewCmdConfigInit(f, func(opts *ConfigInitOptions) error {
		gotOpts = opts
		return nil
	})
	f.IOStreams.In = strings.NewReader("y\n")
	cmd.SetArgs([]string{"--app-id", "x", "--app-secret-stdin", "--lang", "en"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Lang != "en" {
		t.Errorf("expected Lang en, got %s", gotOpts.Lang)
	}
	if !gotOpts.langExplicit {
		t.Error("expected langExplicit=true when --lang is passed")
	}
}

func TestConfigInitCmd_LangDefault(t *testing.T) {
	clearAgentEnv(t) // assumes local workspace; guard refuses init in agent contexts
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *ConfigInitOptions
	cmd := NewCmdConfigInit(f, func(opts *ConfigInitOptions) error {
		gotOpts = opts
		return nil
	})
	f.IOStreams.In = strings.NewReader("y\n")
	cmd.SetArgs([]string{"--app-id", "x", "--app-secret-stdin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Lang != "zh" {
		t.Errorf("expected default Lang zh, got %s", gotOpts.Lang)
	}
	if gotOpts.langExplicit {
		t.Error("expected langExplicit=false when --lang is not passed")
	}
}

func TestHasAnyNonInteractiveFlag(t *testing.T) {
	tests := []struct {
		name string
		opts ConfigInitOptions
		want bool
	}{
		{"empty", ConfigInitOptions{}, false},
		{"new", ConfigInitOptions{New: true}, true},
		{"app-id", ConfigInitOptions{AppID: "x"}, true},
		{"app-secret-stdin", ConfigInitOptions{AppSecretStdin: true}, true},
		{"app-id+secret-stdin", ConfigInitOptions{AppID: "x", AppSecretStdin: true}, true},
		{"lang-only", ConfigInitOptions{Lang: "en"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.hasAnyNonInteractiveFlag()
			if got != tt.want {
				t.Errorf("hasAnyNonInteractiveFlag() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigInitRun_NonTerminal_NoFlags_RejectsWithHint(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// TestFactory has IsTerminal=false by default
	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), Lang: "zh"}
	err := configInitRun(opts)
	if err == nil {
		t.Fatal("expected error for non-terminal without flags")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--new") {
		t.Errorf("expected error to mention --new, got: %s", msg)
	}
	if !strings.Contains(msg, "terminal") {
		t.Errorf("expected error to mention terminal, got: %s", msg)
	}
}

func TestConfigRemoveCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *ConfigRemoveOptions
	cmd := NewCmdConfigRemove(f, func(opts *ConfigRemoveOptions) error {
		gotOpts = opts
		return nil
	})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts == nil {
		t.Fatal("expected opts to be set")
	}
	if gotOpts.Factory != f {
		t.Fatal("expected factory to be preserved in options")
	}
}

func TestConfigRemoveRun_SaveFailurePreservesExistingConfigAndSecrets(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "app-test",
			AppSecret: core.SecretInput{
				Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:app-test"},
			},
			Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_1", UserName: "Tester"}},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	kc := &recordingConfigKeychain{}
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = kc

	// Make subsequent config saves fail while keeping the existing config readable.
	if err := os.Chmod(configDir, 0500); err != nil {
		t.Fatalf("Chmod(%s) error = %v", configDir, err)
	}
	defer os.Chmod(configDir, 0700)

	err := configRemoveRun(&ConfigRemoveOptions{Factory: f})
	if err == nil {
		t.Fatal("expected save failure")
	}
	if !strings.Contains(err.Error(), "failed to save config") {
		t.Fatalf("error = %v, want failed to save config", err)
	}
	if len(kc.removed) != 0 {
		t.Fatalf("expected no keychain cleanup before successful save, got removals: %v", kc.removed)
	}

	// Restore permissions and confirm the original config is still intact.
	if err := os.Chmod(configDir, 0700); err != nil {
		t.Fatalf("restore Chmod(%s) error = %v", configDir, err)
	}
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved == nil || len(saved.Apps) != 1 || saved.Apps[0].AppId != "app-test" {
		t.Fatalf("saved config = %#v, want original single app preserved", saved)
	}
	if got := saved.Apps[0].AppSecret.Ref; got == nil || got.ID != "appsecret:app-test" {
		t.Fatalf("saved app secret ref = %#v, want preserved keychain ref", got)
	}

	configPath := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected existing config file to remain, stat error = %v", err)
	}
}

func TestSaveAsProfile_RejectsProfileNameCollisionWithExistingAppID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{
			{
				Name:      "prod",
				AppId:     "cli_prod",
				AppSecret: core.PlainSecret("secret"),
				Brand:     core.BrandFeishu,
			},
		},
	}

	err := saveAsProfile(existing, keychain.KeychainAccess(&noopConfigKeychain{}), "cli_prod", "app-new", core.PlainSecret("new-secret"), core.BrandLark, "en")
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "conflicts with existing appId") {
		t.Fatalf("error = %v, want conflict with existing appId", err)
	}
}

func TestUpdateExistingProfileWithoutSecret_RejectsAppIDChange(t *testing.T) {
	multi := &core.MultiAppConfig{
		CurrentApp: "prod",
		Apps: []core.AppConfig{
			{
				Name:      "prod",
				AppId:     "app-old",
				AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:app-old"}},
				Brand:     core.BrandFeishu,
				Lang:      "zh",
				Users:     []core.AppUser{{UserOpenId: "ou_1", UserName: "User"}},
			},
		},
	}

	err := updateExistingProfileWithoutSecret(multi, "", "app-new", core.BrandLark, "en")
	if err == nil {
		t.Fatal("expected error when changing app ID without a new secret")
	}
	if !strings.Contains(err.Error(), "App Secret") {
		t.Fatalf("error = %v, want mention of App Secret", err)
	}
}

// stubConfigExtProvider simulates env/sidecar credential mode for config guard tests.
type stubConfigExtProvider struct{ name string }

func (s *stubConfigExtProvider) Name() string { return s.name }
func (s *stubConfigExtProvider) ResolveAccount(_ context.Context) (*extcred.Account, error) {
	return &extcred.Account{AppID: "test-app"}, nil
}
func (s *stubConfigExtProvider) ResolveToken(_ context.Context, _ extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

func newConfigFactoryWithExternalProvider(t *testing.T) *cmdutil.Factory {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	stub := &stubConfigExtProvider{name: "env"}
	cred := credential.NewCredentialProvider([]extcred.Provider{stub}, nil, nil, nil)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Credential = cred
	return f
}

func TestConfigBlockedByExternalProvider(t *testing.T) {
	f := newConfigFactoryWithExternalProvider(t)

	tests := []struct {
		name string
		args []string
	}{
		{"init", []string{"init", "--app-id", "x", "--app-secret-stdin"}},
		{"remove", []string{"remove"}},
		{"show", []string{"show"}},
		{"default-as", []string{"default-as", "user"}},
		{"strict-mode", []string{"strict-mode", "off"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewCmdConfig(f)
			cmd.SilenceErrors = true
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tt.args)

			// Locate the subcommand before execution (PersistentPreRunE receives it as cmd).
			matched, _, _ := cmd.Find(tt.args)

			err := cmd.Execute()

			// PersistentPreRunE sets SilenceUsage on the matched subcommand, not the parent.
			if matched != nil && matched != cmd && !matched.SilenceUsage {
				t.Error("expected PersistentPreRunE to set SilenceUsage on matched subcommand")
			}
			var exitErr *output.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
			}
			if exitErr.Code != output.ExitValidation {
				t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
			}
			if exitErr.Detail == nil || exitErr.Detail.Type != "external_provider" {
				t.Errorf("error type = %v, want %q", exitErr.Detail, "external_provider")
			}
		})
	}
}
