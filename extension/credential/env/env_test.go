// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/envvars"
)

func TestMain(m *testing.M) {
	if err := os.Unsetenv(envvars.CliTenantAccessTokenSource); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

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

type storedTATLookup struct {
	token *credential.Token
	err   error
	calls []string
}

func (l *storedTATLookup) resolve(_ context.Context, appID string) (*credential.Token, error) {
	l.calls = append(l.calls, appID)
	return l.token, l.err
}

func TestStoredTATSourceIsExplicitAndAccountResolutionDoesNotLookup(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliTenantAccessTokenSource, tenantAccessTokenSourceCredentialStore)
	lookup := &storedTATLookup{token: &credential.Token{Value: "stored-tat"}}
	p := (&Provider{}).WithTenantAccessTokenLookup(lookup.resolve)

	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if len(lookup.calls) != 0 {
		t.Fatalf("ResolveAccount() lookup calls = %v, want none", lookup.calls)
	}
	if acct == nil || acct.AppID != "cli_test" || !acct.SupportedIdentities.BotOnly() || acct.DefaultAs != credential.IdentityBot {
		t.Fatalf("ResolveAccount() = %#v, want bot-capable cli_test account", acct)
	}

	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "cli_test"})
	if err != nil || tok == nil || tok.Value != "stored-tat" {
		t.Fatalf("ResolveToken(TAT) = (%#v, %v), want stored-tat", tok, err)
	}
	if len(lookup.calls) != 1 || lookup.calls[0] != "cli_test" {
		t.Fatalf("ResolveToken(TAT) lookup calls = %v, want cli_test", lookup.calls)
	}
}

func TestStoredTATSourceUnsetNeverUsesLookup(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	lookup := &storedTATLookup{token: &credential.Token{Value: "must-not-be-used"}}
	p := (&Provider{}).WithTenantAccessTokenLookup(lookup.resolve)

	_, err := p.ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("ResolveAccount() error = %T %v, want existing APP_ID-only BlockError", err, err)
	}
	if len(lookup.calls) != 0 {
		t.Fatalf("lookup calls = %v, want none without source selector", lookup.calls)
	}
}

func TestStoredTATSourceLeavesUserLaneIndependent(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliUserAccessToken, "env-uat")
	t.Setenv(envvars.CliTenantAccessTokenSource, tenantAccessTokenSourceCredentialStore)
	t.Setenv(envvars.CliDefaultAs, "user")
	lookup := &storedTATLookup{token: &credential.Token{Value: "stored-tat"}}
	p := (&Provider{}).WithTenantAccessTokenLookup(lookup.resolve)

	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct.DefaultAs != credential.IdentityUser || !acct.SupportedIdentities.Has(credential.SupportsUser) || !acct.SupportedIdentities.Has(credential.SupportsBot) {
		t.Fatalf("account = %#v, want user default with user+bot capability", acct)
	}
	if len(lookup.calls) != 0 {
		t.Fatalf("ResolveAccount() lookup calls = %v, want none", lookup.calls)
	}

	uat, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT, AppID: "cli_test"})
	if err != nil || uat == nil || uat.Value != "env-uat" {
		t.Fatalf("ResolveToken(UAT) = (%#v, %v), want env-uat", uat, err)
	}
	if len(lookup.calls) != 0 {
		t.Fatalf("UAT resolution lookup calls = %v, want none", lookup.calls)
	}

	tat, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "cli_test"})
	if err != nil || tat == nil || tat.Value != "stored-tat" {
		t.Fatalf("ResolveToken(TAT) = (%#v, %v), want stored-tat", tat, err)
	}
}

func TestStoredTATSourceStrictUserDoesNotReadStoreDuringAccountResolution(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliStrictMode, "user")
	t.Setenv(envvars.CliTenantAccessTokenSource, tenantAccessTokenSourceCredentialStore)
	lookup := &storedTATLookup{token: &credential.Token{Value: "stored-tat"}}
	p := (&Provider{}).WithTenantAccessTokenLookup(lookup.resolve)

	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if !acct.SupportedIdentities.UserOnly() || acct.DefaultAs != credential.IdentityUser {
		t.Fatalf("account = %#v, want strict user account with user default", acct)
	}
	if len(lookup.calls) != 0 {
		t.Fatalf("ResolveAccount() lookup calls = %v, want none", lookup.calls)
	}
	if tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT, AppID: "cli_test"}); err != nil || tok != nil {
		t.Fatalf("ResolveToken(UAT) = (%#v, %v), want nil without touching TAT store", tok, err)
	}
	if len(lookup.calls) != 0 {
		t.Fatalf("UAT resolution lookup calls = %v, want none", lookup.calls)
	}
}

func TestStoredTATSourcePreservesExplicitUserDefault(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliDefaultAs, "user")
	t.Setenv(envvars.CliTenantAccessTokenSource, tenantAccessTokenSourceCredentialStore)
	lookup := &storedTATLookup{token: &credential.Token{Value: "stored-tat"}}
	p := (&Provider{}).WithTenantAccessTokenLookup(lookup.resolve)

	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct.DefaultAs != credential.IdentityUser || len(lookup.calls) != 0 {
		t.Fatalf("account=%#v lookup=%v, want explicit user default without store read", acct, lookup.calls)
	}
}

func TestStoredTATSourceOverridesLiteralBotTokenLane(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliTenantAccessToken, "env-tat")
	t.Setenv(envvars.CliTenantAccessTokenSource, tenantAccessTokenSourceCredentialStore)
	lookup := &storedTATLookup{token: &credential.Token{Value: "stored-tat"}}
	p := (&Provider{}).WithTenantAccessTokenLookup(lookup.resolve)

	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "cli_test"})
	if err != nil || tok == nil || tok.Value != "stored-tat" {
		t.Fatalf("ResolveToken(TAT) = (%#v, %v), want explicit credential-store source", tok, err)
	}
}

func TestStoredTATSourceValidatesSelectorAndAppID(t *testing.T) {
	t.Run("invalid source", func(t *testing.T) {
		t.Setenv(envvars.CliAppID, "cli_test")
		t.Setenv(envvars.CliTenantAccessTokenSource, "keychain")
		_, err := (&Provider{}).ResolveAccount(context.Background())
		var blockErr *credential.BlockError
		if !errors.As(err, &blockErr) || !strings.Contains(err.Error(), envvars.CliTenantAccessTokenSource) {
			t.Fatalf("error = %T %v, want source BlockError", err, err)
		}
	})

	t.Run("missing app ID", func(t *testing.T) {
		t.Setenv(envvars.CliTenantAccessTokenSource, tenantAccessTokenSourceCredentialStore)
		_, err := (&Provider{}).ResolveAccount(context.Background())
		var blockErr *credential.BlockError
		if !errors.As(err, &blockErr) || !strings.Contains(err.Error(), envvars.CliAppID) {
			t.Fatalf("error = %T %v, want APP_ID BlockError", err, err)
		}
	})
}
