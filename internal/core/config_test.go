// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/keychain"
)

// stubKeychain is a minimal KeychainAccess that always returns ErrNotFound.
type stubKeychain struct{}

func (stubKeychain) Get(service, account string) (string, error) {
	return "", keychain.ErrNotFound
}
func (stubKeychain) Set(service, account, value string) error { return nil }
func (stubKeychain) Remove(service, account string) error     { return nil }

func TestAppConfig_LangSerialization(t *testing.T) {
	app := AppConfig{
		AppId: "cli_test", AppSecret: PlainSecret("secret"),
		Brand: BrandFeishu, Lang: "en", Users: []AppUser{},
	}
	data, err := json.Marshal(app)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got AppConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Lang != "en" {
		t.Errorf("Lang = %q, want %q", got.Lang, "en")
	}
}

func TestAppConfig_LangOmitEmpty(t *testing.T) {
	app := AppConfig{
		AppId: "cli_test", AppSecret: PlainSecret("secret"),
		Brand: BrandFeishu, Users: []AppUser{},
	}
	data, err := json.Marshal(app)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Lang should be omitted when empty
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, exists := raw["lang"]; exists {
		t.Error("expected lang to be omitted when empty")
	}
}

func TestMultiAppConfig_RoundTrip(t *testing.T) {
	config := &MultiAppConfig{
		AuthProxy: &AuthProxyConfig{TrustedHosts: []string{"gate.example.com"}},
		Apps: []AppConfig{{
			AppId: "cli_test", AppSecret: PlainSecret("s"),
			Brand: BrandLark, Lang: "zh", Users: []AppUser{},
		}},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got MultiAppConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(got.Apps))
	}
	if got.Apps[0].Lang != "zh" {
		t.Errorf("Lang = %q, want %q", got.Apps[0].Lang, "zh")
	}
	if got.Apps[0].Brand != BrandLark {
		t.Errorf("Brand = %q, want %q", got.Apps[0].Brand, BrandLark)
	}
	if got.AuthProxy == nil || len(got.AuthProxy.TrustedHosts) != 1 || got.AuthProxy.TrustedHosts[0] != "gate.example.com" {
		t.Errorf("AuthProxy = %#v, want gate.example.com", got.AuthProxy)
	}
}

func TestLoadAuthProxyConfig_ToleratesMissingOrNoAppConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cfg, err := LoadAuthProxyConfig()
	if err != nil {
		t.Fatalf("LoadAuthProxyConfig() missing file error = %v", err)
	}
	if len(cfg.TrustedHosts) != 0 {
		t.Fatalf("TrustedHosts = %#v, want empty", cfg.TrustedHosts)
	}

	if err := SaveMultiAppConfig(&MultiAppConfig{
		AuthProxy: &AuthProxyConfig{TrustedHosts: []string{"gate.example.com"}},
		Apps:      []AppConfig{},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	cfg, err = LoadAuthProxyConfig()
	if err != nil {
		t.Fatalf("LoadAuthProxyConfig() no-app config error = %v", err)
	}
	if len(cfg.TrustedHosts) != 1 || cfg.TrustedHosts[0] != "gate.example.com" {
		t.Fatalf("TrustedHosts = %#v, want gate.example.com", cfg.TrustedHosts)
	}

	if _, err := LoadMultiAppConfig(); err == nil {
		t.Fatal("LoadMultiAppConfig() should still reject no-app config")
	}
}

func TestUpdateAuthProxyConfig_PreservesExistingApps(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := SaveMultiAppConfig(&MultiAppConfig{
		CurrentApp: "default",
		Apps: []AppConfig{{
			Name:      "default",
			AppId:     "cli_test",
			AppSecret: PlainSecret("secret"),
			Brand:     BrandFeishu,
		}},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	if err := UpdateAuthProxyConfig(func(cfg *AuthProxyConfig) {
		cfg.TrustedHosts = append(cfg.TrustedHosts, "gate.example.com")
	}); err != nil {
		t.Fatalf("UpdateAuthProxyConfig() error = %v", err)
	}

	got, err := LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if got.CurrentApp != "default" || len(got.Apps) != 1 || got.Apps[0].AppId != "cli_test" {
		t.Fatalf("app config was not preserved: %#v", got)
	}
	if got.AuthProxy == nil || len(got.AuthProxy.TrustedHosts) != 1 || got.AuthProxy.TrustedHosts[0] != "gate.example.com" {
		t.Fatalf("AuthProxy = %#v, want gate.example.com", got.AuthProxy)
	}
}

func TestUpdateAuthProxyConfig_CreatesConfigWhenMissing(t *testing.T) {
	orig := CurrentWorkspace()
	t.Cleanup(func() { SetCurrentWorkspace(orig) })
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	if err := UpdateAuthProxyConfig(func(cfg *AuthProxyConfig) {
		cfg.TrustedHosts = []string{"gate.example.com"}
	}); err != nil {
		t.Fatalf("UpdateAuthProxyConfig() error = %v", err)
	}

	cfg, err := LoadAuthProxyConfig()
	if err != nil {
		t.Fatalf("LoadAuthProxyConfig() error = %v", err)
	}
	if len(cfg.TrustedHosts) != 1 || cfg.TrustedHosts[0] != "gate.example.com" {
		t.Fatalf("TrustedHosts = %#v, want gate.example.com", cfg.TrustedHosts)
	}

	if _, err := os.Stat(GetConfigPath()); err != nil {
		t.Fatalf("config file was not created: %v", err)
	}
}

func TestAuthProxyConfig_UsesBaseConfigAcrossAgentWorkspaces(t *testing.T) {
	orig := CurrentWorkspace()
	t.Cleanup(func() { SetCurrentWorkspace(orig) })
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	SetCurrentWorkspace(WorkspaceLocal)
	if err := UpdateAuthProxyConfig(func(cfg *AuthProxyConfig) {
		cfg.TrustedHosts = []string{"gate.example.com"}
	}); err != nil {
		t.Fatalf("UpdateAuthProxyConfig() error = %v", err)
	}

	SetCurrentWorkspace(WorkspaceHermes)
	cfg, err := LoadAuthProxyConfig()
	if err != nil {
		t.Fatalf("LoadAuthProxyConfig() error = %v", err)
	}
	if len(cfg.TrustedHosts) != 1 || cfg.TrustedHosts[0] != "gate.example.com" {
		t.Fatalf("TrustedHosts = %#v, want base-config trust from agent workspace", cfg.TrustedHosts)
	}

	if _, err := os.Stat(filepath.Join(GetBaseConfigDir(), "hermes", "config.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("agent workspace config should not be created by auth proxy trust, stat err = %v", err)
	}
}

func TestResolveConfigFromMulti_RejectsSecretKeyMismatch(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{
			{
				AppId: "cli_new_app",
				AppSecret: SecretInput{Ref: &SecretRef{
					Source: "keychain",
					ID:     "appsecret:cli_old_app",
				}},
				Brand: BrandFeishu,
			},
		},
	}

	_, err := ResolveConfigFromMulti(raw, nil, "")
	if err == nil {
		t.Fatal("expected error for mismatched appId and appSecret keychain key")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Hint == "" {
		t.Error("expected non-empty hint in ConfigError")
	}
}

func TestResolveConfigFromMulti_AcceptsPlainSecret(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{
			{
				AppId:     "cli_abc",
				AppSecret: PlainSecret("my-secret"),
				Brand:     BrandFeishu,
			},
		},
	}

	cfg, err := ResolveConfigFromMulti(raw, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AppID != "cli_abc" {
		t.Errorf("AppID = %q, want %q", cfg.AppID, "cli_abc")
	}
}

func TestResolveConfigFromMulti_MatchingKeychainRefPassesValidation(t *testing.T) {
	// Keychain ref matches appId, so validation passes.
	// The subsequent ResolveSecretInput will fail (no real keychain),
	// but that proves the mismatch check itself passed.
	raw := &MultiAppConfig{
		Apps: []AppConfig{
			{
				AppId: "cli_abc",
				AppSecret: SecretInput{Ref: &SecretRef{
					Source: "keychain",
					ID:     "appsecret:cli_abc",
				}},
				Brand: BrandFeishu,
			},
		},
	}

	_, err := ResolveConfigFromMulti(raw, stubKeychain{}, "")
	if err == nil {
		// stubKeychain returns ErrNotFound, so we expect a keychain error,
		// but NOT a mismatch error — that's the point of this test.
		t.Fatal("expected error (keychain entry not found), got nil")
	}
	// The error should come from keychain resolution, NOT from our mismatch check.
	var cfgErr *ConfigError
	if errors.As(err, &cfgErr) {
		if cfgErr.Message == "appId and appSecret keychain key are out of sync" {
			t.Fatal("error came from mismatch check, but keys should match")
		}
	}
}

func TestResolveConfigFromMulti_DoesNotUseEnvProfileFallback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_PROFILE", "missing")

	raw := &MultiAppConfig{
		CurrentApp: "active",
		Apps: []AppConfig{
			{
				Name:      "active",
				AppId:     "cli_active",
				AppSecret: PlainSecret("secret"),
				Brand:     BrandFeishu,
			},
		},
	}

	cfg, err := ResolveConfigFromMulti(raw, nil, "")
	if err != nil {
		t.Fatalf("ResolveConfigFromMulti() error = %v", err)
	}
	if cfg.ProfileName != "active" {
		t.Fatalf("ResolveConfigFromMulti() profile = %q, want %q", cfg.ProfileName, "active")
	}
}

func TestCliConfig_CanBot(t *testing.T) {
	tests := []struct {
		name                string
		supportedIdentities uint8
		want                bool
	}{
		{"unset (0) defaults to true", 0, true},
		{"user only", 1, false},
		{"bot only", 2, true},
		{"both", 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &CliConfig{SupportedIdentities: tt.supportedIdentities}
			if got := cfg.CanBot(); got != tt.want {
				t.Errorf("CanBot() = %v, want %v", got, tt.want)
			}
		})
	}
}
