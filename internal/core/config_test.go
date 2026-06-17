// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
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
	disabled := false
	config := &MultiAppConfig{
		RiskControl: &disabled,
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
	if got.RiskControl == nil || *got.RiskControl {
		t.Errorf("RiskControl = %v, want explicit false", got.RiskControl)
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

	_, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
	if err == nil {
		t.Fatal("expected error for mismatched appId and appSecret keychain key")
	}
	var cfgErr *errs.ConfigError
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

	cfg, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AppID != "cli_abc" {
		t.Errorf("AppID = %q, want %q", cfg.AppID, "cli_abc")
	}
}

func TestResolveConfigFromMulti_CarriesLang(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{
			{
				AppId:     "cli_abc",
				AppSecret: PlainSecret("my-secret"),
				Brand:     BrandFeishu,
				Lang:      "en",
			},
		},
	}

	cfg, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Lang != "en" {
		t.Errorf("Lang = %q, want %q", cfg.Lang, "en")
	}
}

// TestResolveConfigFromMulti_PKJWTSkipsSecretResolution ensures a private_key_jwt
// profile that carries a stale/broken AppSecret ref still resolves cleanly: the
// auth method is judged before any secret handling, so the stale ref is ignored
// instead of producing a confusing secret-resolution failure.
func TestResolveConfigFromMulti_PKJWTSkipsSecretResolution(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_pk",
			// Stale keychain ref whose ID does not match appId — would trip
			// ValidateSecretKeyMatch / ResolveSecretInput if it were reached.
			AppSecret:  SecretInput{Ref: &SecretRef{Source: "keychain", ID: "appsecret:cli_OTHER"}},
			Brand:      BrandFeishu,
			AuthMethod: AuthMethodPrivateKeyJWT,
			KeyRef:     &SecretRef{Source: "tee", ID: "agent-key"},
			Users:      []AppUser{},
		}},
	}
	cfg, err := ResolveConfigFromMulti(raw, stubKeychain{}, "", ProfileFromConfig)
	if err != nil {
		t.Fatalf("pkjwt with stale secret ref must skip secret resolution, got %v", err)
	}
	if cfg.AuthMethod != AuthMethodPrivateKeyJWT || cfg.KeyLabel != "agent-key" {
		t.Errorf("got authMethod=%q keyLabel=%q", cfg.AuthMethod, cfg.KeyLabel)
	}
}

// TestResolveConfigFromMulti_PKJWTRejectsBadKeyRef ensures the stricter keyRef
// check (Source=="tee" && ID!="") rejects malformed handles.
func TestResolveConfigFromMulti_PKJWTRejectsBadKeyRef(t *testing.T) {
	for i, ref := range []*SecretRef{
		{Source: "keychain", ID: "x"}, // wrong source
		{Source: "tee", ID: ""},       // empty id
	} {
		raw := &MultiAppConfig{Apps: []AppConfig{{
			AppId: "cli_pk", Brand: BrandFeishu,
			AuthMethod: AuthMethodPrivateKeyJWT, KeyRef: ref, Users: []AppUser{},
		}}}
		if _, err := ResolveConfigFromMulti(raw, stubKeychain{}, "", ProfileFromConfig); err == nil {
			t.Errorf("case %d: expected ConfigError for bad keyRef", i)
		}
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

	_, err := ResolveConfigFromMulti(raw, stubKeychain{}, "", ProfileFromConfig)
	if err == nil {
		// stubKeychain returns ErrNotFound, so we expect a keychain error,
		// but NOT a mismatch error — that's the point of this test.
		t.Fatal("expected error (keychain entry not found), got nil")
	}
	// The error should come from keychain resolution, NOT from our mismatch check.
	var cfgErr *errs.ConfigError
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

	cfg, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
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

// Runtime configs must never carry raw brand casing: the config ingress
// normalizes it, so downstream equality checks see canonical values.
func TestResolveConfigFromMulti_NormalizesBrand(t *testing.T) {
	multi := &MultiAppConfig{Apps: []AppConfig{{
		AppId:     "cli_x",
		AppSecret: PlainSecret("test-secret"),
		Brand:     LarkBrand(" LARK "),
	}}}
	cfg, err := ResolveConfigFromMulti(multi, nil, "", ProfileFromConfig)
	if err != nil {
		t.Fatalf("ResolveConfigFromMulti error = %v", err)
	}
	if cfg.Brand != BrandLark {
		t.Errorf("Brand = %q, want %q (normalized at ingress)", cfg.Brand, BrandLark)
	}
}

func TestResolveConfigFromMulti_RejectsUnknownAuthMethod(t *testing.T) {
	raw := &MultiAppConfig{Apps: []AppConfig{{
		AppId:      "cli_abc",
		AppSecret:  PlainSecret("my-secret"),
		Brand:      BrandFeishu,
		AuthMethod: "bogus_method",
	}}}

	_, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
	if err == nil {
		t.Fatal("expected error for unknown authMethod")
	}
	var configErr *errs.ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("expected ConfigError, got %T: %v", err, err)
	}
}

func TestResolveConfigFromMulti_PrivateKeyJWTRequiresKeyRef(t *testing.T) {
	raw := &MultiAppConfig{Apps: []AppConfig{{
		AppId:      "cli_abc",
		Brand:      BrandFeishu,
		AuthMethod: AuthMethodPrivateKeyJWT,
	}}}

	_, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
	if err == nil {
		t.Fatal("expected error for private_key_jwt without keyRef")
	}
	var configErr *errs.ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("expected ConfigError, got %T: %v", err, err)
	}

	raw.Apps[0].KeyRef = &SecretRef{Source: "tee", ID: "larksuite-cli-agent"}
	cfg, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
	if err != nil {
		t.Fatalf("unexpected error with keyRef present: %v", err)
	}
	if cfg.KeyLabel != "larksuite-cli-agent" {
		t.Errorf("KeyLabel = %q, want larksuite-cli-agent", cfg.KeyLabel)
	}
}
