// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"errors"
	"testing"

	_ "github.com/larksuite/cli/extension/credential/env"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
)

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

	f := NewDefault(InvocationContext{Profile: "target"})
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

	f := NewDefault(InvocationContext{Profile: "missing"})
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeOff {
		t.Fatalf("ResolveStrictMode() = %q, want %q", got, core.StrictModeOff)
	}
	_, err := f.Config()
	if err == nil {
		t.Fatal("Config() error = nil, want non-nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("Config() error type = %T, want *core.ConfigError", err)
	}
	if cfgErr.Message != `profile "missing" not found` {
		t.Fatalf("Config() error message = %q, want %q", cfgErr.Message, `profile "missing" not found`)
	}
}

func TestBuildSDKTransport_IncludesRetryTransport(t *testing.T) {
	transport := buildSDKTransport()

	sec, ok := transport.(*internalauth.SecurityPolicyTransport)
	if !ok {
		t.Fatalf("outer transport type = %T, want *auth.SecurityPolicyTransport", transport)
	}
	ua, ok := sec.Base.(*UserAgentTransport)
	if !ok {
		t.Fatalf("middle transport type = %T, want *UserAgentTransport", sec.Base)
	}
	if _, ok := ua.Base.(*RetryTransport); !ok {
		t.Fatalf("inner transport type = %T, want *RetryTransport", ua.Base)
	}
}

func TestNewDefault_ResolveAs_UsesDefaultAsFromEnvAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "env-secret")
	t.Setenv(envvars.CliDefaultAs, "user")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(InvocationContext{})
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

	f := NewDefault(InvocationContext{})

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

	f := NewDefault(InvocationContext{})

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
