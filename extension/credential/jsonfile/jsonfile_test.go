// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package jsonfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/envvars"
)

func writeTempJSON(t *testing.T, data map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0o600)
	return path
}

func TestProvider_Name(t *testing.T) {
	if (&Provider{}).Name() != "jsonfile" {
		t.Fail()
	}
}

func TestResolveAccount_BothSet(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id":     "cli_test",
		"app_secret": "secret_test",
		"brand":      "feishu",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.AppID != "cli_test" || acct.AppSecret != "secret_test" || acct.Brand != "feishu" {
		t.Errorf("unexpected: %+v", acct)
	}
}

func TestResolveAccount_EnvNotSet(t *testing.T) {
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil || acct != nil {
		t.Errorf("expected nil, nil; got %+v, %v", acct, err)
	}
}

func TestResolveAccount_OnlyIDSet(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id": "cli_test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_OnlySecretSet(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_secret": "secret_test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_AppIDAndUserTokenWithoutSecret(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id":            "cli_test",
		"user_access_token": "uat_test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

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

func TestResolveAccount_DefaultBrand(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id":     "cli_test",
		"app_secret": "secret_test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	acct, _ := (&Provider{}).ResolveAccount(context.Background())
	if acct.Brand != "feishu" {
		t.Errorf("expected 'feishu', got %q", acct.Brand)
	}
}

func TestResolveAccount_DefaultAsFromFile(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id":     "cli_test",
		"app_secret": "secret_test",
		"default_as": "user",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.DefaultAs != "user" {
		t.Errorf("expected default-as user, got %q", acct.DefaultAs)
	}
}

func TestResolveAccount_InvalidDefaultAsRejected(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id":     "app",
		"app_secret": "secret",
		"default_as": "invalid",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	_, err := (&Provider{}).ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid default-as")
	}
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %T", err)
	}
}

func TestResolveAccount_InferFromUATOnly(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id":            "app",
		"app_secret":        "secret",
		"user_access_token": "u-tok",
	})
	t.Setenv(envvars.CliCredentialFile, path)

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
	path := writeTempJSON(t, map[string]string{
		"app_id":              "app",
		"app_secret":          "secret",
		"tenant_access_token": "t-tok",
	})
	t.Setenv(envvars.CliCredentialFile, path)

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
	path := writeTempJSON(t, map[string]string{
		"app_id":              "app",
		"app_secret":          "secret",
		"user_access_token":   "u-tok",
		"tenant_access_token": "t-tok",
	})
	t.Setenv(envvars.CliCredentialFile, path)

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

func TestResolveToken_UATSet(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"user_access_token": "u-file",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "u-file" || tok.Source != "jsonfile:"+path {
		t.Errorf("unexpected: %+v", tok)
	}
}

func TestResolveToken_TATSet(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"tenant_access_token": "t-file",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "t-file" || tok.Source != "jsonfile:"+path {
		t.Errorf("unexpected: %+v", tok)
	}
}

func TestResolveToken_NotSet(t *testing.T) {
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil || tok != nil {
		t.Errorf("expected nil, nil; got %+v, %v", tok, err)
	}
}

func TestResolveToken_TokenEmpty(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"user_access_token": "",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil || tok != nil {
		t.Errorf("expected nil, nil; got %+v, %v", tok, err)
	}
}

func TestResolveAccount_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envvars.CliCredentialFile, filepath.Join(dir, "nonexistent.json"))

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	os.WriteFile(path, []byte("{not json}"), 0o600)
	t.Setenv(envvars.CliCredentialFile, path)

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_OnlyTokenSetWithoutAppID(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"user_access_token": "uat_test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_RelativePathRejected(t *testing.T) {
	t.Setenv(envvars.CliCredentialFile, "relative/path/cred.json")

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_RefreshTokenSetsUserSupport(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id":        "app",
		"app_secret":    "secret",
		"refresh_token": "rt-test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.UserOnly() {
		t.Errorf("expected user-only from refresh_token, got %d", acct.SupportedIdentities)
	}
	if acct.DefaultAs != "user" {
		t.Errorf("expected default-as user from refresh_token, got %q", acct.DefaultAs)
	}
}

func TestResolveAccount_RefreshTokenWithoutAppSecret(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id":        "app",
		"refresh_token": "rt-test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_RefreshTokenWithoutAppID(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"refresh_token": "rt-test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveToken_UATPreferredOverRefreshToken(t *testing.T) {
	path := writeTempJSON(t, map[string]string{
		"app_id":            "app",
		"app_secret":        "secret",
		"user_access_token": "u-direct",
		"refresh_token":     "rt-test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "u-direct" {
		t.Errorf("expected direct UAT, got %q", tok.Value)
	}
}

func TestResolveToken_RefreshTokenSuccess(t *testing.T) {
	refreshMu.Lock()
	refreshCache = nil
	refreshMu.Unlock()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("grant_type") != "refresh_token" {
			http.Error(w, "bad grant_type", 400)
			return
		}
		if r.FormValue("refresh_token") != "rt-valid" {
			http.Error(w, "bad refresh_token", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"u-refreshed","expires_in":7200}`)
	}))
	defer srv.Close()

	orig := tokenEndpointFunc
	tokenEndpointFunc = func(_ string) string { return srv.URL + "/open-apis/authen/v2/oauth/token" }
	defer func() { tokenEndpointFunc = orig }()

	path := writeTempJSON(t, map[string]string{
		"app_id":        "app",
		"app_secret":    "secret",
		"refresh_token": "rt-valid",
		"brand":         "feishu",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "u-refreshed" {
		t.Errorf("expected refreshed token, got %q", tok.Value)
	}
	if tok.Source != "jsonfile:"+path+":refreshed" {
		t.Errorf("unexpected source: %s", tok.Source)
	}
}

func TestResolveToken_RefreshTokenCached(t *testing.T) {
	refreshMu.Lock()
	refreshCache = &cachedToken{
		value:     "u-cached",
		expiresAt: time.Now().Add(time.Hour),
	}
	refreshMu.Unlock()
	defer func() {
		refreshMu.Lock()
		refreshCache = nil
		refreshMu.Unlock()
	}()

	path := writeTempJSON(t, map[string]string{
		"app_id":        "app",
		"app_secret":    "secret",
		"refresh_token": "rt-test",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "u-cached" {
		t.Errorf("expected cached token, got %q", tok.Value)
	}
}

func TestResolveToken_RefreshTokenError(t *testing.T) {
	refreshMu.Lock()
	refreshCache = nil
	refreshMu.Unlock()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"error":"invalid_grant","error_description":"refresh token expired"}`)
	}))
	defer srv.Close()

	orig := tokenEndpointFunc
	tokenEndpointFunc = func(_ string) string { return srv.URL + "/open-apis/authen/v2/oauth/token" }
	defer func() { tokenEndpointFunc = orig }()

	path := writeTempJSON(t, map[string]string{
		"app_id":        "app",
		"app_secret":    "secret",
		"refresh_token": "rt-expired",
		"brand":         "feishu",
	})
	t.Setenv(envvars.CliCredentialFile, path)

	_, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}
