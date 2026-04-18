// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

package sidecar

import (
	"context"
	"os"
	"testing"

	"github.com/larksuite/cli/extension/credential"
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

func TestResolveAccount_MissingProxyKey(t *testing.T) {
	setEnv(t, envvars.CliAuthProxy, "http://127.0.0.1:16384")
	unsetEnv(t, envvars.CliProxyKey)
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

func TestResolveAccount_MissingAppID(t *testing.T) {
	setEnv(t, envvars.CliAuthProxy, "http://127.0.0.1:16384")
	setEnv(t, envvars.CliProxyKey, "test-key")
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
