// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package env

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestResolveAccount_AppIDFileAndUserTokenFileWithoutSecret(t *testing.T) {
	t.Setenv(envvars.CliAppIDFile, writeEnvFile(t, "app_id", "cli_file\n"))
	t.Setenv(envvars.CliUserAccessTokenFile, writeEnvFile(t, "uat", "uat_file\n"))

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct == nil {
		t.Fatal("expected account, got nil")
	}
	if acct.AppID != "cli_file" {
		t.Fatalf("AppID = %q, want cli_file", acct.AppID)
	}
	if acct.AppSecret != credential.NoAppSecret {
		t.Fatalf("AppSecret = %q, want credential.NoAppSecret", acct.AppSecret)
	}
	if !acct.SupportedIdentities.UserOnly() {
		t.Fatalf("expected user-only identity support, got %d", acct.SupportedIdentities)
	}
	if acct.DefaultAs != "user" {
		t.Fatalf("DefaultAs = %q, want user", acct.DefaultAs)
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

func TestResolveAccount_OnlyTokenFileSetWithoutAppID(t *testing.T) {
	t.Setenv(envvars.CliUserAccessTokenFile, writeEnvFile(t, "uat", "uat_test"))

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
	if !strings.Contains(err.Error(), envvars.CliAppIDFile) {
		t.Fatalf("error = %v, want mention of %s", err, envvars.CliAppIDFile)
	}
}

func TestResolveAccount_EnvAndFileConflictRejected(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_env")
	t.Setenv(envvars.CliAppIDFile, writeEnvFile(t, "app_id", "cli_file"))

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
	if !strings.Contains(err.Error(), "set only one of "+envvars.CliAppID+" or "+envvars.CliAppIDFile) {
		t.Fatalf("error = %v, want conflict message", err)
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

func TestResolveToken_UATFileSet(t *testing.T) {
	t.Setenv(envvars.CliUserAccessTokenFile, writeEnvFile(t, "uat", "u-file\n"))
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "u-file" || tok.Source != "file:"+envvars.CliUserAccessTokenFile {
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

func TestResolveToken_TATFileSet(t *testing.T) {
	t.Setenv(envvars.CliTenantAccessTokenFile, writeEnvFile(t, "tat", "t-file\n"))
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "t-file" || tok.Source != "file:"+envvars.CliTenantAccessTokenFile {
		t.Errorf("unexpected: %+v", tok)
	}
}

func TestResolveToken_NotSet(t *testing.T) {
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil || tok != nil {
		t.Errorf("expected nil, nil; got %+v, %v", tok, err)
	}
}

func TestResolveToken_EmptyFileRejected(t *testing.T) {
	t.Setenv(envvars.CliUserAccessTokenFile, writeEnvFile(t, "uat", "\n"))
	_, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
	if !strings.Contains(err.Error(), "file is empty") {
		t.Fatalf("error = %v, want empty file message", err)
	}
}

func TestResolveToken_MultilineFileRejected(t *testing.T) {
	t.Setenv(envvars.CliUserAccessTokenFile, writeEnvFile(t, "uat", "one\ntwo"))
	_, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
	if !strings.Contains(err.Error(), "exactly one credential value") {
		t.Fatalf("error = %v, want multiline file message", err)
	}
}

func TestResolveToken_RelativeFileRejected(t *testing.T) {
	t.Setenv(envvars.CliUserAccessTokenFile, "relative-token")
	_, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("error = %v, want absolute path message", err)
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

func writeEnvFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
