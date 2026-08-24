// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/envvars"
)

func TestProvider_Name(t *testing.T) {
	if (&Provider{}).Name() != "env" {
		t.Fail()
	}
}

func TestResolveAccount_BothSet(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliAppSecret, "secret_test")
	t.Setenv(envvars.CliBrand, " LARK ")

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.AppID != "cli_test" || acct.AppSecret != "secret_test" || acct.Brand != "lark" {
		t.Errorf("unexpected: %+v", acct)
	}
}

func TestResolveAccount_NeitherSet(t *testing.T) {
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil || acct != nil {
		t.Errorf("expected nil, nil; got %+v, %v", acct, err)
	}
}

func TestResolveAccount_OnlyIDSet(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_AppIDAndUserTokenWithoutSecret(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliUserAccessToken, "uat_test")

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct == nil {
		t.Fatal("expected account, got nil")
	}
	if acct.AppSecret != credential.NoAppSecret {
		t.Fatalf("AppSecret = %q, want credential.NoAppSecret", acct.AppSecret)
	}
	if acct.AppID != "cli_test" {
		t.Fatalf("AppID = %q, want cli_test", acct.AppID)
	}
}

func TestResolveAccount_OnlySecretSet(t *testing.T) {
	t.Setenv(envvars.CliAppSecret, "secret_test")
	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_OnlyTokenSetWithoutAppID(t *testing.T) {
	t.Setenv(envvars.CliUserAccessToken, "uat_test")

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
	if !strings.Contains(err.Error(), envvars.CliAppID) {
		t.Fatalf("error = %v, want mention of %s", err, envvars.CliAppID)
	}
}

func TestResolveAccount_DefaultBrand(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliAppSecret, "secret_test")
	acct, _ := (&Provider{}).ResolveAccount(context.Background())
	if acct.Brand != "feishu" {
		t.Errorf("expected 'feishu', got %q", acct.Brand)
	}
}

func TestResolveAccount_DefaultAsFromEnv(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliAppSecret, "secret_test")
	t.Setenv(envvars.CliDefaultAs, "user")

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.DefaultAs != "user" {
		t.Errorf("expected default-as user, got %q", acct.DefaultAs)
	}
}

func TestResolveToken_UATSet(t *testing.T) {
	t.Setenv(envvars.CliUserAccessToken, "u-env")
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "u-env" || tok.Source != "env:"+envvars.CliUserAccessToken {
		t.Errorf("unexpected: %+v", tok)
	}
}

func TestResolveToken_TATSet(t *testing.T) {
	t.Setenv(envvars.CliTenantAccessToken, "t-env")
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "t-env" || tok.Source != "env:"+envvars.CliTenantAccessToken {
		t.Errorf("unexpected: %+v", tok)
	}
}

func TestResolveToken_NotSet(t *testing.T) {
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil || tok != nil {
		t.Errorf("expected nil, nil; got %+v, %v", tok, err)
	}
}

func TestResolveAccount_StrictModeBot(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliStrictMode, "bot")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.BotOnly() {
		t.Errorf("expected bot-only, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_StrictModeUser(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliStrictMode, "user")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.UserOnly() {
		t.Errorf("expected user-only, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_StrictModeOff(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliStrictMode, "off")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.SupportedIdentities != credential.SupportsAll {
		t.Errorf("expected SupportsAll, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_InferFromUATOnly(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliUserAccessToken, "u-tok")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.UserOnly() {
		t.Errorf("expected user-only from UAT inference, got %d", acct.SupportedIdentities)
	}
	if acct.DefaultAs != "user" {
		t.Errorf("expected default-as user from UAT inference, got %q", acct.DefaultAs)
	}
}

func TestResolveAccount_InferFromTATOnly(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliTenantAccessToken, "t-tok")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.BotOnly() {
		t.Errorf("expected bot-only from TAT inference, got %d", acct.SupportedIdentities)
	}
	if acct.DefaultAs != "bot" {
		t.Errorf("expected default-as bot from TAT inference, got %q", acct.DefaultAs)
	}
}

func TestResolveAccount_InferBothTokens(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliUserAccessToken, "u-tok")
	t.Setenv(envvars.CliTenantAccessToken, "t-tok")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.SupportedIdentities != credential.SupportsAll {
		t.Errorf("expected SupportsAll, got %d", acct.SupportedIdentities)
	}
	if acct.DefaultAs != "user" {
		t.Errorf("expected default-as user when both tokens are present, got %q", acct.DefaultAs)
	}
}

func TestResolveAccount_StrictModeOverridesTokenInference(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliUserAccessToken, "u-tok")
	t.Setenv(envvars.CliTenantAccessToken, "t-tok")
	t.Setenv(envvars.CliStrictMode, "bot")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.BotOnly() {
		t.Errorf("strict mode should override token inference, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_InvalidStrictModeRejected(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliStrictMode, "invalid")

	_, err := (&Provider{}).ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid strict mode")
	}
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %T", err)
	}
	if !strings.Contains(err.Error(), envvars.CliStrictMode) {
		t.Fatalf("error = %v, want mention of %s", err, envvars.CliStrictMode)
	}
}

func TestResolveAccount_InvalidDefaultAsRejected(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliDefaultAs, "invalid")

	_, err := (&Provider{}).ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid default-as")
	}
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %T", err)
	}
	if !strings.Contains(err.Error(), envvars.CliDefaultAs) {
		t.Fatalf("error = %v, want mention of %s", err, envvars.CliDefaultAs)
	}
}

type envTokenFallback struct {
	token *credential.Token
	err   error
	calls []credential.TokenSpec
}

func (f *envTokenFallback) Name() string { return "injected-tat" }

func (f *envTokenFallback) ResolveAccount(context.Context) (*credential.Account, error) {
	return nil, nil
}

func (f *envTokenFallback) ResolveToken(_ context.Context, req credential.TokenSpec) (*credential.Token, error) {
	f.calls = append(f.calls, req)
	return f.token, f.err
}

func TestResolveAccount_AppIDOnlyUsesInjectedTAT(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	fallback := &envTokenFallback{token: &credential.Token{Value: "stored-tat"}}
	p := (&Provider{}).WithTokenFallback(fallback)

	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct == nil || acct.AppID != "cli_test" {
		t.Fatalf("ResolveAccount() = %#v, want cli_test account", acct)
	}
	if !acct.SupportedIdentities.BotOnly() || acct.DefaultAs != credential.IdentityBot {
		t.Fatalf("account identity = (supported=%d, default=%q), want bot-only/bot", acct.SupportedIdentities, acct.DefaultAs)
	}
	if len(fallback.calls) != 1 || fallback.calls[0].AppID != "cli_test" || fallback.calls[0].Type != credential.TokenTypeTAT {
		t.Fatalf("fallback calls = %+v, want one cli_test TAT lookup", fallback.calls)
	}
}

func TestResolveAccount_AppIDOnlyInjectedTATRespectsStrictMode(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliStrictMode, "user")
	fallback := &envTokenFallback{token: &credential.Token{Value: "stored-tat"}}
	p := (&Provider{}).WithTokenFallback(fallback)

	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if !acct.SupportedIdentities.UserOnly() {
		t.Fatalf("SupportedIdentities = %d, want explicit user-only strict mode", acct.SupportedIdentities)
	}
}

func TestResolveAccount_AppIDOnlyWithoutInjectedTATStillBlocks(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	p := (&Provider{}).WithTokenFallback(&envTokenFallback{})

	_, err := p.ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("ResolveAccount() error = %T %v, want BlockError", err, err)
	}
}

func TestResolveToken_TATPrefersEnvironmentOverInjected(t *testing.T) {
	t.Setenv(envvars.CliTenantAccessToken, "env-tat")
	fallback := &envTokenFallback{token: &credential.Token{Value: "stored-tat"}}
	p := (&Provider{}).WithTokenFallback(fallback)

	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "cli_test"})
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if tok == nil || tok.Value != "env-tat" {
		t.Fatalf("ResolveToken() = %#v, want env-tat", tok)
	}
	if len(fallback.calls) != 0 {
		t.Fatalf("fallback calls = %+v, want none while env TAT is set", fallback.calls)
	}
}

func TestResolveToken_TATFallsBackToInjected(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	fallback := &envTokenFallback{token: &credential.Token{Value: "stored-tat", Source: "keychain:tat:cli_test"}}
	p := (&Provider{}).WithTokenFallback(fallback)

	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "cli_test"})
	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}
	if tok == nil || tok.Value != "stored-tat" || tok.Source != "keychain:tat:cli_test" {
		t.Fatalf("ResolveToken() = %#v, want injected token", tok)
	}
}

func TestResolveToken_TATFallbackRequiresSelectedAppID(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_selected")
	fallback := &envTokenFallback{token: &credential.Token{Value: "stored-tat"}}
	p := (&Provider{}).WithTokenFallback(fallback)

	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "cli_other"})
	if err != nil || tok != nil {
		t.Fatalf("ResolveToken() = (%#v, %v), want nil, nil for non-selected app ID", tok, err)
	}
	if len(fallback.calls) != 0 {
		t.Fatalf("fallback calls = %+v, want none for non-selected app ID", fallback.calls)
	}
}

func TestResolveAccount_PropagatesInjectedTATStorageError(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	sentinel := errors.New("storage failed")
	p := (&Provider{}).WithTokenFallback(&envTokenFallback{err: sentinel})

	_, err := p.ResolveAccount(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("ResolveAccount() error = %v, want storage failure", err)
	}
}

func TestResolveAccount_ExistingCredentialDoesNotConsultInjectedTAT(t *testing.T) {
	tests := []struct {
		name      string
		appSecret string
		uat       string
	}{
		{name: "app secret", appSecret: "app-secret"},
		{name: "user token", uat: "user-token"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envvars.CliAppID, "cli_test")
			t.Setenv(envvars.CliAppSecret, tc.appSecret)
			t.Setenv(envvars.CliUserAccessToken, tc.uat)
			fallback := &envTokenFallback{err: errors.New("must not be called")}
			p := (&Provider{}).WithTokenFallback(fallback)

			acct, err := p.ResolveAccount(context.Background())
			if err != nil || acct == nil {
				t.Fatalf("ResolveAccount() = (%#v, %v), want existing credential account", acct, err)
			}
			if len(fallback.calls) != 0 {
				t.Fatalf("fallback calls = %+v, want none", fallback.calls)
			}
		})
	}
}

func TestResolveToken_ExistingCredentialDoesNotConsultInjectedTAT(t *testing.T) {
	tests := []struct {
		name      string
		appSecret string
		uat       string
	}{
		{name: "app secret", appSecret: "app-secret"},
		{name: "user token", uat: "user-token"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envvars.CliAppID, "cli_test")
			t.Setenv(envvars.CliAppSecret, tc.appSecret)
			t.Setenv(envvars.CliUserAccessToken, tc.uat)
			fallback := &envTokenFallback{token: &credential.Token{Value: "stored-tat"}}
			p := (&Provider{}).WithTokenFallback(fallback)

			tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "cli_test"})
			if err != nil || tok != nil {
				t.Fatalf("ResolveToken() = (%#v, %v), want existing behavior nil, nil", tok, err)
			}
			if len(fallback.calls) != 0 {
				t.Fatalf("fallback calls = %+v, want none", fallback.calls)
			}
		})
	}
}
