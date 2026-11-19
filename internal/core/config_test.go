// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/vfs"
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

func TestSigningKeyRefConfigJSON(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	fileID, err := json.Marshal(filepath.Join(t.TempDir(), "private.pem"))
	require.NoError(t, err)
	for _, input := range []string{
		`{"source":"tee","provider":"macos-keychain","id":"managed-key"}`,
		`{"source":"tee","provider":"macos-secure-enclave","id":"enclave-key"}`,
		`{"source":"file","id":` + string(fileID) + `}`,
	} {
		var ref KeyRef
		require.NoError(t, json.Unmarshal([]byte(input), &ref))
		app := AppConfig{AppId: "cli_test", AuthMethod: AuthMethodPrivateKeyJWTLocalKeyPair, KeyRef: &ref}
		require.NoError(t, SaveMultiAppConfig(&MultiAppConfig{Apps: []AppConfig{app}}))

		data, err := vfs.ReadFile(GetConfigPath())
		require.NoError(t, err)
		var disk struct {
			Apps []map[string]json.RawMessage `json:"apps"`
		}
		require.NoError(t, json.Unmarshal(data, &disk))
		require.Len(t, disk.Apps, 1)
		require.JSONEq(t, `"private_key_jwt_local_keypair"`, string(disk.Apps[0]["authMethod"]))
		require.JSONEq(t, input, string(disk.Apps[0]["keyRef"]))

		loaded, err := LoadMultiAppConfig()
		require.NoError(t, err)
		cfg, err := ResolveConfigFromMulti(loaded, nil, "", ProfileFromConfig)
		require.NoError(t, err)
		require.Equal(t, ref, KeyRef{Source: cfg.KeySource, Provider: cfg.KeyProvider, ID: cfg.KeyLabel})
	}
}

func TestAppSecretReferenceSources(t *testing.T) {
	for _, input := range []string{
		`{"source":"file","id":"/path/to/app-secret"}`,
		`{"source":"keychain","id":"appsecret:cli_test","provider":"reserved"}`,
	} {
		var secret SecretInput
		require.NoError(t, json.Unmarshal([]byte(input), &secret))
		data, err := json.Marshal(secret)
		require.NoError(t, err)
		require.JSONEq(t, input, string(data))
	}
	var secret SecretInput
	require.Error(t, json.Unmarshal([]byte(`{"source":"tee","provider":"macos-keychain","id":"managed-key"}`), &secret))
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

func TestResolveSigningKeyProviders(t *testing.T) {
	for _, provider := range []string{
		keysigner.SoftwareSignerName,
		keysigner.MacOSKeychainSignerName, keysigner.MacOSSecureEnclaveSignerName,
		keysigner.WindowsPlatformKSPSignerName, keysigner.WindowsSoftwareKSPSignerName, keysigner.LinuxTPMSignerName,
	} {
		ref := KeyRef{Source: KeySourceTEE, Provider: provider, ID: "key-1"}
		raw := &MultiAppConfig{Apps: []AppConfig{{
			AppId: "cli_pk", AuthMethod: AuthMethodPrivateKeyJWTLocalKeyPair, KeyRef: &ref,
			// Resolving this stale app secret would fail the App ID check.
			AppSecret: SecretInput{Ref: &SecretRef{Source: "keychain", ID: "appsecret:cli_OTHER"}},
		}}}
		cfg, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
		require.NoError(t, err, "provider %q", provider)
		require.Equal(t, AuthMethodPrivateKeyJWTLocalKeyPair, cfg.AuthMethod)
		require.Equal(t, ref, KeyRef{Source: cfg.KeySource, Provider: cfg.KeyProvider, ID: cfg.KeyLabel})
	}
}

func TestResolveConfigRejectsInvalidKeyRef(t *testing.T) {
	file := filepath.Join(t.TempDir(), "private.pem")
	for _, ref := range []KeyRef{
		{Source: "keychain", ID: "key"},
		{Source: KeySourceTEE, Provider: keysigner.SoftwareSignerName},
		{Source: KeySourceTEE, ID: "key"},
		{Source: KeySourceTEE, ID: "key", Provider: "unknown.provider"},
		{Source: KeySourceTEE, ID: "key", Provider: " macos-keychain "},
		{Source: KeySourceTEE, ID: "key", Provider: "MacOS-Keychain"},
		{Source: KeySourceTEE, ID: "key", Provider: "larksuite.keyless"},
		{Source: KeySourceFile},
		{Source: KeySourceFile, ID: "./private.pem"},
		{Source: KeySourceFile, ID: "~/private.pem"},
		{Source: KeySourceFile, ID: file, Provider: keysigner.SoftwareSignerName},
		{Source: KeySourceFile, ID: file, Provider: " "},
	} {
		raw := &MultiAppConfig{Apps: []AppConfig{{AppId: "cli_pk", AuthMethod: AuthMethodPrivateKeyJWTLocalKeyPair, KeyRef: &ref}}}
		_, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
		var configErr *errs.ConfigError
		require.ErrorAs(t, err, &configErr, "keyRef %+v", ref)
		require.Equal(t, errs.SubtypeInvalidConfig, configErr.Subtype)
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

func TestResolveConfigFromMulti_PrivateKeyJWTLocalKeyPairRequiresKeyRef(t *testing.T) {
	raw := &MultiAppConfig{Apps: []AppConfig{{AppId: "cli_abc", AuthMethod: AuthMethodPrivateKeyJWTLocalKeyPair}}}
	_, err := ResolveConfigFromMulti(raw, nil, "", ProfileFromConfig)
	var configErr *errs.ConfigError
	require.ErrorAs(t, err, &configErr)
	require.Equal(t, errs.SubtypeInvalidConfig, configErr.Subtype)
	require.Contains(t, configErr.Hint, "lark-cli config init --private-key-jwt")
	require.Contains(t, configErr.Hint, "existing-app")
	require.Contains(t, configErr.Hint, "cli_abc")
	require.NotContains(t, configErr.Hint, "--new")
}
