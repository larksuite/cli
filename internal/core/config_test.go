// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
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
}

// noopKeychain satisfies keychain.KeychainAccess for tests (returns empty, no error).
type noopKeychain struct{}

func (n *noopKeychain) Get(service, account string) (string, error) { return "", nil }
func (n *noopKeychain) Set(service, account, value string) error    { return nil }
func (n *noopKeychain) Remove(service, account string) error        { return nil }

func TestFindAppByID_Found(t *testing.T) {
	apps := []AppConfig{
		{AppId: "cli_aaa", Brand: BrandFeishu},
		{AppId: "cli_bbb", Brand: BrandLark},
	}
	got, err := FindAppByID(apps, "cli_bbb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AppId != "cli_bbb" {
		t.Errorf("AppId = %q, want %q", got.AppId, "cli_bbb")
	}
}

func TestFindAppByID_NotFound(t *testing.T) {
	apps := []AppConfig{
		{AppId: "cli_aaa", Brand: BrandFeishu},
	}
	_, err := FindAppByID(apps, "cli_zzz")
	if err == nil {
		t.Fatal("expected error for missing app")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected ConfigError, got %T", err)
	}
}

func TestActiveApp_EnvVar(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_PROFILE", "cli_bbb")
	multi := &MultiAppConfig{
		Apps: []AppConfig{
			{AppId: "cli_aaa"},
			{AppId: "cli_bbb"},
		},
	}
	got, err := ActiveApp(multi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AppId != "cli_bbb" {
		t.Errorf("AppId = %q, want %q", got.AppId, "cli_bbb")
	}
}

func TestActiveApp_FallsBackToFirst(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_PROFILE", "")
	multi := &MultiAppConfig{
		Apps: []AppConfig{
			{AppId: "cli_aaa"},
			{AppId: "cli_bbb"},
		},
	}
	got, err := ActiveApp(multi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AppId != "cli_aaa" {
		t.Errorf("AppId = %q, want %q", got.AppId, "cli_aaa")
	}
}

func TestActiveApp_FallsBackToCurrentApp(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_PROFILE", "")
	multi := &MultiAppConfig{
		CurrentApp: "cli_bbb",
		Apps: []AppConfig{
			{AppId: "cli_aaa"},
			{AppId: "cli_bbb"},
		},
	}
	got, err := ActiveApp(multi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AppId != "cli_bbb" {
		t.Errorf("AppId = %q, want %q", got.AppId, "cli_bbb")
	}
}

func TestActiveApp_EnvVarNotFound(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_PROFILE", "cli_missing")
	multi := &MultiAppConfig{
		Apps: []AppConfig{{AppId: "cli_aaa"}},
	}
	_, err := ActiveApp(multi)
	if err == nil {
		t.Fatal("expected error for missing app")
	}
}

func TestRequireConfig_EnvVarSelectsApp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_PROFILE", "cli_bbb")

	config := &MultiAppConfig{
		Apps: []AppConfig{
			{AppId: "cli_aaa", AppSecret: PlainSecret("sec_a"), Brand: BrandFeishu, Users: []AppUser{}},
			{AppId: "cli_bbb", AppSecret: PlainSecret("sec_b"), Brand: BrandLark, Users: []AppUser{{UserOpenId: "ou_123", UserName: "bob"}}},
		},
	}
	if err := SaveMultiAppConfig(config); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := RequireConfig(&noopKeychain{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AppID != "cli_bbb" {
		t.Errorf("AppID = %q, want %q", got.AppID, "cli_bbb")
	}
	if got.UserOpenId != "ou_123" {
		t.Errorf("UserOpenId = %q, want %q", got.UserOpenId, "ou_123")
	}
}

func TestRequireConfig_DefaultsToFirstApp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_PROFILE", "")

	config := &MultiAppConfig{
		Apps: []AppConfig{
			{AppId: "cli_first", AppSecret: PlainSecret("sec"), Brand: BrandFeishu, Users: []AppUser{}},
		},
	}
	if err := SaveMultiAppConfig(config); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := RequireConfig(&noopKeychain{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AppID != "cli_first" {
		t.Errorf("AppID = %q, want %q", got.AppID, "cli_first")
	}
}

func TestRequireConfig_EnvVarNotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_PROFILE", "cli_missing")

	config := &MultiAppConfig{
		Apps: []AppConfig{
			{AppId: "cli_aaa", AppSecret: PlainSecret("sec"), Brand: BrandFeishu, Users: []AppUser{}},
		},
	}
	if err := SaveMultiAppConfig(config); err != nil {
		t.Fatalf("save: %v", err)
	}

	_, err := RequireConfig(&noopKeychain{})
	if err == nil {
		t.Fatal("expected error for missing app")
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

func TestResolveConfigFromMulti_ProfileOverrideTakesPrecedenceOverEnv(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_PROFILE", "env-profile")

	raw := &MultiAppConfig{
		Apps: []AppConfig{
			{
				Name:      "env-profile",
				AppId:     "cli_env",
				AppSecret: PlainSecret("secret_env"),
				Brand:     BrandFeishu,
			},
			{
				Name:      "explicit",
				AppId:     "cli_explicit",
				AppSecret: PlainSecret("secret_explicit"),
				Brand:     BrandFeishu,
			},
		},
	}

	// When profileOverride is set, it should be used instead of the env var.
	cfg, err := ResolveConfigFromMulti(raw, nil, "explicit")
	if err != nil {
		t.Fatalf("ResolveConfigFromMulti() error = %v", err)
	}
	if cfg.ProfileName != "explicit" {
		t.Fatalf("ResolveConfigFromMulti() profile = %q, want %q", cfg.ProfileName, "explicit")
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
