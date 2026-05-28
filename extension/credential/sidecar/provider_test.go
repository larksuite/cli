// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

package sidecar

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/sidecar"
)

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	os.Setenv(key, value)
	t.Cleanup(func() {
		if hadOld {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if hadOld {
			os.Setenv(key, old)
		}
	})
}

func trustRemoteProxy(t *testing.T, hosts ...string) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		AuthProxy: &core.AuthProxyConfig{TrustedHosts: hosts},
		Apps: []core.AppConfig{{
			AppId:     "cli_existing",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.BrandFeishu,
		}},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
}

func TestResolveAccount_NotActive(t *testing.T) {
	unsetEnv(t, envvars.CliAuthProxy)

	p := &Provider{}
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct != nil {
		t.Fatal("expected nil account when AUTH_PROXY not set")
	}
}

func TestResolveAccount_Active(t *testing.T) {
	setEnv(t, envvars.CliAuthProxy, "http://127.0.0.1:16384")
	setEnv(t, envvars.CliProxyKey, "test-key")
	unsetEnv(t, envvars.CliProxySession)
	setEnv(t, envvars.CliAppID, "cli_test123")
	setEnv(t, envvars.CliBrand, "lark")
	unsetEnv(t, envvars.CliDefaultAs)
	unsetEnv(t, envvars.CliStrictMode)

	p := &Provider{}
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct == nil {
		t.Fatal("expected non-nil account")
	}
	if acct.AppID != "cli_test123" {
		t.Errorf("AppID = %q, want %q", acct.AppID, "cli_test123")
	}
	if acct.Brand != credential.BrandLark {
		t.Errorf("Brand = %q, want %q", acct.Brand, credential.BrandLark)
	}
	if acct.AppSecret != credential.NoAppSecret {
		t.Errorf("AppSecret should be NoAppSecret, got %q", acct.AppSecret)
	}
	if acct.SupportedIdentities != credential.SupportsAll {
		t.Errorf("SupportedIdentities = %d, want %d (SupportsAll)", acct.SupportedIdentities, credential.SupportsAll)
	}
}

func TestResolveAccount_RemoteHTTPSActive(t *testing.T) {
	trustRemoteProxy(t, "auth-proxy.example.com")
	setEnv(t, envvars.CliAuthProxy, "https://auth-proxy.example.com")
	setEnv(t, envvars.CliProxyKey, "proxy-signing-key")
	setEnv(t, envvars.CliProxySession, "proxy-session")
	setEnv(t, envvars.CliAppID, "cli_test123")
	unsetEnv(t, envvars.CliBrand)
	unsetEnv(t, envvars.CliDefaultAs)
	unsetEnv(t, envvars.CliStrictMode)

	p := &Provider{}
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct == nil {
		t.Fatal("expected non-nil account")
	}
	if acct.AppID != "cli_test123" {
		t.Errorf("AppID = %q, want %q", acct.AppID, "cli_test123")
	}
	if acct.Brand != credential.BrandFeishu {
		t.Errorf("Brand = %q, want %q", acct.Brand, credential.BrandFeishu)
	}
	if acct.AppSecret != credential.NoAppSecret {
		t.Errorf("AppSecret should be NoAppSecret, got %q", acct.AppSecret)
	}
	if acct.SupportedIdentities != credential.SupportsAll {
		t.Errorf("SupportedIdentities = %d, want %d (SupportsAll)", acct.SupportedIdentities, credential.SupportsAll)
	}
}

func TestResolveAccount_RemoteHTTPSUntrustedProxy(t *testing.T) {
	trustRemoteProxy(t, "trusted-proxy.example.com")
	setEnv(t, envvars.CliAuthProxy, "https://evil.example.com")
	setEnv(t, envvars.CliProxyKey, "proxy-signing-key")
	setEnv(t, envvars.CliProxySession, "proxy-session")
	setEnv(t, envvars.CliAppID, "cli_test")

	p := &Provider{}
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error when remote proxy host is not trusted")
	}
	if _, ok := err.(*credential.BlockError); !ok {
		t.Fatalf("expected BlockError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("error should mention trust failure, got: %v", err)
	}
}

func TestResolveAccount_MissingProxyKey(t *testing.T) {
	setEnv(t, envvars.CliAuthProxy, "http://127.0.0.1:16384")
	unsetEnv(t, envvars.CliProxyKey)
	unsetEnv(t, envvars.CliProxySession)
	setEnv(t, envvars.CliAppID, "cli_test")

	p := &Provider{}
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error when PROXY_KEY is missing")
	}
	if _, ok := err.(*credential.BlockError); !ok {
		t.Fatalf("expected BlockError, got %T: %v", err, err)
	}
}

func TestResolveAccount_RemoteHTTPSMissingProxySession(t *testing.T) {
	trustRemoteProxy(t, "auth-proxy.example.com")
	setEnv(t, envvars.CliAuthProxy, "https://auth-proxy.example.com")
	setEnv(t, envvars.CliProxyKey, "proxy-signing-key")
	unsetEnv(t, envvars.CliProxySession)
	setEnv(t, envvars.CliAppID, "cli_test")

	p := &Provider{}
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error when PROXY_SESSION is missing")
	}
	if _, ok := err.(*credential.BlockError); !ok {
		t.Fatalf("expected BlockError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), envvars.CliProxySession) {
		t.Fatalf("error should mention %s, got: %v", envvars.CliProxySession, err)
	}
}

func TestResolveAccount_RemoteHTTPSMissingProxyKey(t *testing.T) {
	trustRemoteProxy(t, "auth-proxy.example.com")
	setEnv(t, envvars.CliAuthProxy, "https://auth-proxy.example.com")
	unsetEnv(t, envvars.CliProxyKey)
	setEnv(t, envvars.CliProxySession, "proxy-session")
	setEnv(t, envvars.CliAppID, "cli_test")

	p := &Provider{}
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error when remote proxy signing key is missing")
	}
	if _, ok := err.(*credential.BlockError); !ok {
		t.Fatalf("expected BlockError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), envvars.CliProxyKey) {
		t.Fatalf("error should mention %s, got: %v", envvars.CliProxyKey, err)
	}
}

func TestResolveAccount_MissingAppID(t *testing.T) {
	setEnv(t, envvars.CliAuthProxy, "http://127.0.0.1:16384")
	setEnv(t, envvars.CliProxyKey, "test-key")
	unsetEnv(t, envvars.CliProxySession)
	unsetEnv(t, envvars.CliAppID)

	p := &Provider{}
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error when APP_ID is missing")
	}
	if _, ok := err.(*credential.BlockError); !ok {
		t.Fatalf("expected BlockError, got %T: %v", err, err)
	}
}

func TestResolveAccount_StrictMode(t *testing.T) {
	setEnv(t, envvars.CliAuthProxy, "http://127.0.0.1:16384")
	setEnv(t, envvars.CliProxyKey, "test-key")
	setEnv(t, envvars.CliAppID, "cli_test")

	tests := []struct {
		mode string
		want credential.IdentitySupport
	}{
		{"bot", credential.SupportsBot},
		{"user", credential.SupportsUser},
		{"off", credential.SupportsAll},
		{"", credential.SupportsAll},
	}

	p := &Provider{}
	for _, tt := range tests {
		t.Run("strict_"+tt.mode, func(t *testing.T) {
			if tt.mode == "" {
				unsetEnv(t, envvars.CliStrictMode)
			} else {
				setEnv(t, envvars.CliStrictMode, tt.mode)
			}
			acct, err := p.ResolveAccount(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if acct.SupportedIdentities != tt.want {
				t.Errorf("SupportedIdentities = %d, want %d", acct.SupportedIdentities, tt.want)
			}
		})
	}
}

func TestResolveToken_NotActive(t *testing.T) {
	unsetEnv(t, envvars.CliAuthProxy)

	p := &Provider{}
	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != nil {
		t.Fatal("expected nil token when AUTH_PROXY not set")
	}
}

func TestResolveToken_Sentinels(t *testing.T) {
	setEnv(t, envvars.CliAuthProxy, "http://127.0.0.1:16384")
	setEnv(t, envvars.CliProxyKey, "test-key")

	p := &Provider{}

	// UAT
	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatalf("UAT: unexpected error: %v", err)
	}
	if tok.Value != sidecar.SentinelUAT {
		t.Errorf("UAT value = %q, want %q", tok.Value, sidecar.SentinelUAT)
	}
	if tok.Scopes != "" {
		t.Errorf("UAT scopes should be empty, got %q", tok.Scopes)
	}

	// TAT
	tok, err = p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT})
	if err != nil {
		t.Fatalf("TAT: unexpected error: %v", err)
	}
	if tok.Value != sidecar.SentinelTAT {
		t.Errorf("TAT value = %q, want %q", tok.Value, sidecar.SentinelTAT)
	}
}
