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
