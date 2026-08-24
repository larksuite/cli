// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	_ "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

type countingFileIOProvider struct {
	resolveCalls int
}

type fallbackConfigurableProvider struct {
	name       string
	configured bool
}

func (p *fallbackConfigurableProvider) Name() string { return p.name }
func (p *fallbackConfigurableProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return nil, nil
}
func (p *fallbackConfigurableProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}
func (p *fallbackConfigurableProvider) WithTokenFallback(extcred.Provider) extcred.Provider {
	clone := *p
	clone.configured = true
	return &clone
}

type inertCredentialProvider struct{ name string }

func (p *inertCredentialProvider) Name() string { return p.name }
func (p *inertCredentialProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return nil, nil
}
func (p *inertCredentialProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

func TestWithInjectedTATFallback_ConfiguresOnlyEnvProvider(t *testing.T) {
	envProvider := &fallbackConfigurableProvider{name: "env"}
	thirdParty := &fallbackConfigurableProvider{name: "third-party"}
	fallback := &inertCredentialProvider{name: "injected-tat"}

	got := withInjectedTATFallback([]extcred.Provider{envProvider, thirdParty}, fallback)
	if configured, ok := got[0].(*fallbackConfigurableProvider); !ok || !configured.configured {
		t.Fatalf("env provider = %#v, want configured copy", got[0])
	}
	if got[1] != thirdParty || thirdParty.configured {
		t.Fatalf("third-party provider = %#v, want original unconfigured provider", got[1])
	}
}

type factoryInjectedTATKeychain struct {
	value    string
	getCalls int
}

func (k *factoryInjectedTATKeychain) Get(service, account string) (string, error) {
	k.getCalls++
	if service == keychain.LarkCliService && account == "tat:env-app" {
		return k.value, nil
	}
	return "", nil
}

func (k *factoryInjectedTATKeychain) Set(string, string, string) error { return nil }
func (k *factoryInjectedTATKeychain) Remove(string, string) error      { return nil }

func (p *countingFileIOProvider) Name() string { return "counting" }

func (p *countingFileIOProvider) ResolveFileIO(context.Context) fileio.FileIO {
	p.resolveCalls++
	return &localfileio.LocalFileIO{}
}

func TestNewDefault_InvocationProfileUsedByStrictModeAndConfig(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")

	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	bot := core.StrictModeBot
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "app-default",
				AppSecret: core.PlainSecret("secret-default"),
				Brand:     core.BrandFeishu,
			},
			{
				Name:       "target",
				AppId:      "app-target",
				AppSecret:  core.PlainSecret("secret-target"),
				Brand:      core.BrandFeishu,
				StrictMode: &bot,
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f := NewDefault(nil, InvocationContext{Profile: "target"})
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeBot {
		t.Fatalf("ResolveStrictMode() = %q, want %q", got, core.StrictModeBot)
	}
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if cfg.ProfileName != "target" {
		t.Fatalf("Config() profile = %q, want %q", cfg.ProfileName, "target")
	}
	if cfg.AppID != "app-target" {
		t.Fatalf("Config() appID = %q, want %q", cfg.AppID, "app-target")
	}
}

func TestNewDefault_InvocationProfileMissingSticksAcrossEarlyStrictMode(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")

	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "app-default",
				AppSecret: core.PlainSecret("secret-default"),
				Brand:     core.BrandFeishu,
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f := NewDefault(nil, InvocationContext{Profile: "missing"})
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeOff {
		t.Fatalf("ResolveStrictMode() = %q, want %q", got, core.StrictModeOff)
	}
	_, err := f.Config()
	if err == nil {
		t.Fatal("Config() error = nil, want non-nil")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("Config() error type = %T, want *errs.ConfigError", err)
	}
	if cfgErr.Message != `profile "missing" not found` {
		t.Fatalf("Config() error message = %q, want %q", cfgErr.Message, `profile "missing" not found`)
	}
}

func TestNewDefault_ResolveAs_UsesDefaultAsFromEnvAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "env-secret")
	t.Setenv(envvars.CliDefaultAs, "user")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(nil, InvocationContext{})
	cmd := newCmdWithAsFlag("auto", false)

	got := f.ResolveAs(context.Background(), cmd, "auto")
	if got != core.AsUser {
		t.Fatalf("ResolveAs() = %q, want %q", got, core.AsUser)
	}
	if f.IdentityAutoDetected {
		t.Fatal("IdentityAutoDetected = true, want false")
	}
}

func TestNewDefault_ConfigReturnsCliConfigCopyOfCredentialAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "env-secret")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliUserAccessToken, "uat-token")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(nil, InvocationContext{})

	acct, err := f.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}

	cfg.AppID = "mutated-cli-config"
	if acct.AppID != "env-app" {
		t.Fatalf("credential account mutated via Config(): got %q, want %q", acct.AppID, "env-app")
	}
}

func TestNewDefault_ConfigUsesRuntimePlaceholderForTokenOnlyEnvAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliUserAccessToken, "uat-token")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(nil, InvocationContext{})

	acct, err := f.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct.AppSecret != "" {
		t.Fatalf("credential account AppSecret = %q, want empty string", acct.AppSecret)
	}

	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if cfg.AppSecret != "" {
		t.Fatalf("Config().AppSecret = %q, want empty string for token-only account", cfg.AppSecret)
	}
	if credential.HasRealAppSecret(cfg.AppSecret) {
		t.Fatalf("Config().AppSecret = %q, want token-only no-secret marker", cfg.AppSecret)
	}
}

func TestNewDefault_EnvAppIDUsesInjectedTATFromFactoryKeychain(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv(envvars.CliStrictMode, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(nil, InvocationContext{})
	kc := &factoryInjectedTATKeychain{value: "stored-tenant-token"}
	f.Keychain = kc

	acct, err := f.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct.AppID != "env-app" || acct.DefaultAs != core.AsBot {
		t.Fatalf("account = %#v, want env-app with bot default", acct)
	}
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeBot {
		t.Fatalf("ResolveStrictMode() = %q, want bot", got)
	}
	result, err := f.Credential.ResolveToken(context.Background(), credential.TokenSpec{
		Type:  credential.TokenTypeTAT,
		AppID: "env-app",
	})
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if result == nil || result.Token != "stored-tenant-token" {
		t.Fatalf("ResolveToken() = %#v, want stored token", result)
	}
	if result.Source != core.CredentialSourceEnv {
		t.Fatalf("credential source = %q, want env provider context", result.Source)
	}
	if kc.getCalls != 1 {
		t.Fatalf("Keychain Get calls = %d, want one shared cached lookup", kc.getCalls)
	}
}

func TestNewDefault_FileIOProviderDoesNotResolveDuringInitialization(t *testing.T) {
	prev := fileio.GetProvider()
	provider := &countingFileIOProvider{}
	fileio.Register(provider)
	t.Cleanup(func() { fileio.Register(prev) })

	f := NewDefault(nil, InvocationContext{})
	if f.FileIOProvider != provider {
		t.Fatalf("NewDefault() provider = %T, want %T", f.FileIOProvider, provider)
	}
	if provider.resolveCalls != 0 {
		t.Fatalf("ResolveFileIO() calls after NewDefault() = %d, want 0", provider.resolveCalls)
	}

	if got := f.ResolveFileIO(context.Background()); got == nil {
		t.Fatal("ResolveFileIO() = nil, want non-nil")
	}
	if provider.resolveCalls != 1 {
		t.Fatalf("ResolveFileIO() calls after explicit resolve = %d, want 1", provider.resolveCalls)
	}
}
