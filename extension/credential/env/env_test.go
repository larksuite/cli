// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"errors"
	"slices"
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
	if blockErr.Code != credential.BlockReasonCredentialIncomplete {
		t.Fatalf("Code = %q, want %q", blockErr.Code, credential.BlockReasonCredentialIncomplete)
	}
	want := []string{envvars.CliAppSecret, envvars.CliUserAccessToken, envvars.CliTenantAccessToken}
	if !slices.Equal(blockErr.RequiredAnyOf, want) {
		t.Fatalf("RequiredAnyOf = %v, want %v", blockErr.RequiredAnyOf, want)
	}
	if len(blockErr.MissingKeys) != 0 {
		t.Fatalf("MissingKeys = %v, want empty", blockErr.MissingKeys)
	}
	if !slices.Equal(blockErr.PresentKeys, []string{envvars.CliAppID}) {
		t.Fatalf("PresentKeys = %v, want [%s]", blockErr.PresentKeys, envvars.CliAppID)
	}
	if blockErr.AppID != "cli_test" {
		t.Fatalf("AppID = %q, want cli_test", blockErr.AppID)
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
	if blockErr.Code != credential.BlockReasonCredentialIncomplete ||
		!slices.Equal(blockErr.MissingKeys, []string{envvars.CliAppID}) ||
		!slices.Equal(blockErr.PresentKeys, []string{envvars.CliAppSecret}) {
		t.Fatalf("BlockError = %+v, want incomplete with missing APP_ID and present APP_SECRET", blockErr)
	}
	if len(blockErr.RequiredAnyOf) != 0 {
		t.Fatalf("RequiredAnyOf = %v, want empty for APP_SECRET-only", blockErr.RequiredAnyOf)
	}
}

func TestResolveAccount_OnlyTokenSetWithoutAppID(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
	}{
		{name: "UAT", key: envvars.CliUserAccessToken},
		{name: "TAT", key: envvars.CliTenantAccessToken},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envvars.CliAppID, "")
			t.Setenv(envvars.CliAppSecret, "")
			t.Setenv(envvars.CliUserAccessToken, "")
			t.Setenv(envvars.CliTenantAccessToken, "")
			t.Setenv(tt.key, "token_test")

			_, err := (&Provider{}).ResolveAccount(context.Background())
			var blockErr *credential.BlockError
			if !errors.As(err, &blockErr) {
				t.Fatalf("expected BlockError, got %v", err)
			}
			if !strings.Contains(err.Error(), envvars.CliAppID) {
				t.Fatalf("error = %v, want mention of %s", err, envvars.CliAppID)
			}
			if blockErr.Code != credential.BlockReasonCredentialIncomplete ||
				!slices.Equal(blockErr.MissingKeys, []string{envvars.CliAppID}) ||
				!slices.Equal(blockErr.PresentKeys, []string{tt.key}) {
				t.Fatalf("BlockError = %+v, want incomplete for %s", blockErr, tt.key)
			}
			if len(blockErr.RequiredAnyOf) != 0 {
				t.Fatalf("RequiredAnyOf = %v, want empty for %s-only", blockErr.RequiredAnyOf, tt.name)
			}
		})
	}
}

func TestResolveAccount_InvalidPolicyRejectedBeforeIncomplete(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
	}{
		{name: "DEFAULT_AS", key: envvars.CliDefaultAs},
		{name: "STRICT_MODE", key: envvars.CliStrictMode},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envvars.CliAppID, "cli_test")
			t.Setenv(envvars.CliAppSecret, "")
			t.Setenv(envvars.CliUserAccessToken, "")
			t.Setenv(envvars.CliTenantAccessToken, "")
			t.Setenv(tt.key, "banana")

			_, err := (&Provider{}).ResolveAccount(context.Background())
			var blockErr *credential.BlockError
			if !errors.As(err, &blockErr) {
				t.Fatalf("error = %T %v, want BlockError", err, err)
			}
			if blockErr.Code != credential.BlockReasonInvalidPolicy {
				t.Fatalf("Code = %q, want %q", blockErr.Code, credential.BlockReasonInvalidPolicy)
			}
			if blockErr.Param != tt.key {
				t.Fatalf("Param = %q, want %q", blockErr.Param, tt.key)
			}
			if !strings.Contains(blockErr.Reason, tt.key) {
				t.Fatalf("reason = %q, want %s", blockErr.Reason, tt.key)
			}
		})
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
	if blockErr.Code != credential.BlockReasonInvalidPolicy || blockErr.Param != envvars.CliStrictMode {
		t.Fatalf("BlockError = %+v, want invalid_policy with Param %s", blockErr, envvars.CliStrictMode)
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
	if blockErr.Code != credential.BlockReasonInvalidPolicy || blockErr.Param != envvars.CliDefaultAs {
		t.Fatalf("BlockError = %+v, want invalid_policy with Param %s", blockErr, envvars.CliDefaultAs)
	}
	if !strings.Contains(err.Error(), envvars.CliDefaultAs) {
		t.Fatalf("error = %v, want mention of %s", err, envvars.CliDefaultAs)
	}
}
