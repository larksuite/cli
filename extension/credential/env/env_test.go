// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/envvars"
)

// stubTATFetcher replaces tatFetcher for the duration of t and counts calls.
func stubTATFetcher(t *testing.T, fn func(ctx context.Context, hc *http.Client, brand credential.Brand, appID, appSecret string) (string, int, error)) *int {
	t.Helper()
	orig := tatFetcher
	calls := 0
	tatFetcher = func(ctx context.Context, hc *http.Client, brand credential.Brand, appID, appSecret string) (string, int, error) {
		calls++
		return fn(ctx, hc, brand, appID, appSecret)
	}
	t.Cleanup(func() { tatFetcher = orig })
	return &calls
}

func TestProvider_Name(t *testing.T) {
	if (&Provider{}).Name() != "env" {
		t.Fail()
	}
}

func TestResolveAccount_BothSet(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_test")
	t.Setenv(envvars.CliAppSecret, "secret_test")
	t.Setenv(envvars.CliBrand, "feishu")

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.AppID != "cli_test" || acct.AppSecret != "secret_test" || acct.Brand != "feishu" {
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
	// app_secret alone is enough to mint a tenant token, so bot is also supported.
	if acct.SupportedIdentities != credential.SupportsAll {
		t.Errorf("expected SupportsAll (user from UAT + bot from app_secret), got %d", acct.SupportedIdentities)
	}
	if acct.DefaultAs != "user" {
		t.Errorf("expected default-as user from UAT inference, got %q", acct.DefaultAs)
	}
}

func TestResolveAccount_InferFromUATWithoutSecret(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliUserAccessToken, "u-tok")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.UserOnly() {
		t.Errorf("expected user-only when only UAT is available, got %d", acct.SupportedIdentities)
	}
	if acct.DefaultAs != "user" {
		t.Errorf("expected default-as user, got %q", acct.DefaultAs)
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

func TestResolveAccount_InferSupportsBotFromAppSecret(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.Has(credential.SupportsBot) {
		t.Errorf("expected SupportsBot to be inferred from app_secret, got %d", acct.SupportedIdentities)
	}
	if acct.DefaultAs != credential.IdentityBot {
		t.Errorf("expected DefaultAs=bot when only app_secret is available, got %q", acct.DefaultAs)
	}
}

func TestResolveToken_TATMintedFromAppSecret(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")

	calls := stubTATFetcher(t, func(_ context.Context, _ *http.Client, brand credential.Brand, appID, appSecret string) (string, int, error) {
		if appID != "app" || appSecret != "secret" {
			t.Fatalf("unexpected creds passed to fetcher: appID=%q appSecret=%q", appID, appSecret)
		}
		if brand != credential.BrandFeishu {
			t.Fatalf("unexpected brand: %q", brand)
		}
		return "minted-tat", 7200, nil
	})

	p := &Provider{}
	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if tok == nil || tok.Value != "minted-tat" {
		t.Fatalf("unexpected token: %+v", tok)
	}
	if *calls != 1 {
		t.Errorf("expected 1 fetch call, got %d", *calls)
	}

	// Second call within TTL should hit the cache, not refetch.
	tok2, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if tok2.Value != "minted-tat" {
		t.Errorf("cached value mismatch: %+v", tok2)
	}
	if *calls != 1 {
		t.Errorf("cache miss: expected 1 fetch call total, got %d", *calls)
	}
}

func TestResolveToken_TATPreferredOverMintWhenEnvSet(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")
	t.Setenv(envvars.CliTenantAccessToken, "env-tat")

	calls := stubTATFetcher(t, func(_ context.Context, _ *http.Client, _ credential.Brand, _, _ string) (string, int, error) {
		t.Fatal("fetcher should not be called when TAT env var is set")
		return "", 0, nil
	})

	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "env-tat" {
		t.Errorf("expected env TAT, got %q", tok.Value)
	}
	if *calls != 0 {
		t.Errorf("expected 0 fetcher calls, got %d", *calls)
	}
}

func TestResolveToken_TATRefetchAfterExpiry(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")

	p := &Provider{}
	calls := stubTATFetcher(t, func(_ context.Context, _ *http.Client, _ credential.Brand, _, _ string) (string, int, error) {
		return "fresh", 1, nil
	})

	if _, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "app"}); err != nil {
		t.Fatal(err)
	}
	// Force cache expiry by rewinding the stored expiry.
	p.tatMu.Lock()
	p.tatCache.expiresAt = time.Now().Add(-time.Second)
	p.tatMu.Unlock()

	if _, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "app"}); err != nil {
		t.Fatal(err)
	}
	if *calls != 2 {
		t.Errorf("expected 2 fetch calls across cache expiry, got %d", *calls)
	}
}

func TestResolveToken_TATRequestedAppIDMismatchBlocks(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliAppSecret, "secret")

	stubTATFetcher(t, func(_ context.Context, _ *http.Client, _ credential.Brand, _, _ string) (string, int, error) {
		t.Fatal("fetcher should not be called on app_id mismatch")
		return "", 0, nil
	})

	_, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "other"})
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveToken_TATReturnsNilWhenSecretMissing(t *testing.T) {
	t.Setenv(envvars.CliAppID, "app")
	t.Setenv(envvars.CliUserAccessToken, "u-tok") // keeps ResolveAccount valid, but no secret for mint

	stubTATFetcher(t, func(_ context.Context, _ *http.Client, _ credential.Brand, _, _ string) (string, int, error) {
		t.Fatal("fetcher should not be called without app_secret")
		return "", 0, nil
	})

	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT, AppID: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if tok != nil {
		t.Errorf("expected nil token when app_secret missing, got %+v", tok)
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
