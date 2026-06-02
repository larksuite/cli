// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package secplugin

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	internalsec "github.com/larksuite/cli/internal/secplugin"
)

// TestProvider_Metadata verifies the registered provider metadata.
func TestProvider_Metadata(t *testing.T) {
	p := &Provider{}
	if p.Name() != "secplugin" {
		t.Fatalf("Name() = %q, want secplugin", p.Name())
	}
	if p.Priority() != 1 {
		t.Fatalf("Priority() = %d, want 1", p.Priority())
	}
}

// TestProvider_UsesSecConfigDefaults verifies that SEC config defaults populate
// the placeholder account when env vars are absent.
func TestProvider_UsesSecConfigDefaults(t *testing.T) {
	prev := loadSecConfig
	t.Cleanup(func() { loadSecConfig = prev })

	loadSecConfig = func() (*internalsec.Config, error) {
		return &internalsec.Config{
			Enable:     true,
			Auth:       true,
			AppID:      "cli_test_app",
			Brand:      "lark",
			DefaultAs:  "bot",
			StrictMode: "bot",
		}, nil
	}

	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliBrand, "")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliStrictMode, "")

	p := &Provider{}
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct == nil {
		t.Fatal("ResolveAccount() = nil, want account")
	}
	if acct.AppID != "cli_test_app" {
		t.Fatalf("acct.AppID = %q, want %q", acct.AppID, "cli_test_app")
	}
	if string(acct.Brand) != "lark" {
		t.Fatalf("acct.Brand = %q, want %q", acct.Brand, "lark")
	}
	if string(acct.DefaultAs) != "bot" {
		t.Fatalf("acct.DefaultAs = %q, want %q", acct.DefaultAs, "bot")
	}
	// StrictMode=bot => SupportsBot only.
	if acct.SupportedIdentities != 2 {
		t.Fatalf("acct.SupportedIdentities = %d, want %d (SupportsBot)", acct.SupportedIdentities, 2)
	}
}

// TestProvider_EnvOverridesSecConfigDefaults verifies that explicit environment
// variables override SEC config defaults.
func TestProvider_EnvOverridesSecConfigDefaults(t *testing.T) {
	prev := loadSecConfig
	t.Cleanup(func() { loadSecConfig = prev })

	loadSecConfig = func() (*internalsec.Config, error) {
		return &internalsec.Config{
			Enable:     true,
			Auth:       true,
			AppID:      "cli_test_app",
			Brand:      "feishu",
			DefaultAs:  "bot",
			StrictMode: "bot",
		}, nil
	}

	t.Setenv(envvars.CliAppID, "cli_env_app")
	t.Setenv(envvars.CliBrand, "lark")
	t.Setenv(envvars.CliDefaultAs, "user")
	t.Setenv(envvars.CliStrictMode, "user")

	p := &Provider{}
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct == nil {
		t.Fatal("ResolveAccount() = nil, want account")
	}
	if acct.AppID != "cli_env_app" {
		t.Fatalf("acct.AppID = %q, want %q", acct.AppID, "cli_env_app")
	}
	if string(acct.Brand) != "lark" {
		t.Fatalf("acct.Brand = %q, want %q", acct.Brand, "lark")
	}
	if string(acct.DefaultAs) != "user" {
		t.Fatalf("acct.DefaultAs = %q, want %q", acct.DefaultAs, "user")
	}
	// StrictMode=user => SupportsUser only (bit 1).
	if acct.SupportedIdentities != 1 {
		t.Fatalf("acct.SupportedIdentities = %d, want %d (SupportsUser)", acct.SupportedIdentities, 1)
	}
}

// TestProvider_ResolveAccount_ReturnsNilWhenDisabled verifies early nil returns
// when SEC_AUTH mode is unavailable.
func TestProvider_ResolveAccount_ReturnsNilWhenDisabled(t *testing.T) {
	prev := loadSecConfig
	t.Cleanup(func() { loadSecConfig = prev })

	cases := []struct {
		name string
		cfg  *internalsec.Config
	}{
		{name: "nil config", cfg: nil},
		{name: "auth disabled", cfg: &internalsec.Config{Enable: true, Auth: false}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			loadSecConfig = func() (*internalsec.Config, error) { return tt.cfg, nil }
			acct, err := (&Provider{}).ResolveAccount(context.Background())
			if err != nil {
				t.Fatalf("ResolveAccount() error = %v", err)
			}
			if acct != nil {
				t.Fatalf("ResolveAccount() = %#v, want nil", acct)
			}
		})
	}
}

// TestProvider_ResolveAccount_LoadErrorBlocks verifies that SEC config load failures
// stop provider resolution.
func TestProvider_ResolveAccount_LoadErrorBlocks(t *testing.T) {
	prev := loadSecConfig
	t.Cleanup(func() { loadSecConfig = prev })

	loadSecConfig = func() (*internalsec.Config, error) {
		return nil, context.DeadlineExceeded
	}

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("ResolveAccount() error = nil, want block error")
	}
	if acct != nil {
		t.Fatalf("ResolveAccount() = %#v, want nil", acct)
	}
	blockErr, ok := err.(*credential.BlockError)
	if !ok {
		t.Fatalf("ResolveAccount() error = %T, want *credential.BlockError", err)
	}
	if blockErr.Provider != "secplugin" {
		t.Fatalf("blockErr.Provider = %q, want secplugin", blockErr.Provider)
	}
	if !strings.Contains(blockErr.Reason, context.DeadlineExceeded.Error()) {
		t.Fatalf("blockErr.Reason = %q, want load error text", blockErr.Reason)
	}
}

// TestProvider_ResolveAccount_DefaultsBrandAndSupport verifies fallback defaults
// for brand and supported identities.
func TestProvider_ResolveAccount_DefaultsBrandAndSupport(t *testing.T) {
	prev := loadSecConfig
	t.Cleanup(func() { loadSecConfig = prev })

	loadSecConfig = func() (*internalsec.Config, error) {
		return &internalsec.Config{
			Enable: true,
			Auth:   true,
		}, nil
	}

	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliBrand, "")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliStrictMode, "")
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "app_from_disk",
			AppSecret: core.PlainSecret("secret"),
			DefaultAs: core.AsBot,
		}},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct == nil {
		t.Fatal("ResolveAccount() = nil, want account")
	}
	if acct.Brand != credential.BrandFeishu {
		t.Fatalf("acct.Brand = %q, want %q", acct.Brand, credential.BrandFeishu)
	}
	if acct.SupportedIdentities != credential.SupportsAll {
		t.Fatalf("acct.SupportedIdentities = %d, want %d", acct.SupportedIdentities, credential.SupportsAll)
	}
	if acct.DefaultAs != credential.Identity("bot") {
		t.Fatalf("acct.DefaultAs = %q, want bot", acct.DefaultAs)
	}
	if acct.AppID != "app_from_disk" {
		t.Fatalf("acct.AppID = %q, want app_from_disk", acct.AppID)
	}
}

// TestProvider_ResolveAccount_InvalidValuesBlock verifies validation failures for
// brand and identity-related settings.
func TestProvider_ResolveAccount_InvalidValuesBlock(t *testing.T) {
	prev := loadSecConfig
	t.Cleanup(func() { loadSecConfig = prev })

	cases := []struct {
		name     string
		cfg      *internalsec.Config
		envKey   string
		envValue string
		want     string
	}{
		{
			name: "invalid brand from config",
			cfg:  &internalsec.Config{Enable: true, Auth: true, AppID: "cli_test_app", Brand: "bad-brand"},
			want: "invalid " + envvars.CliBrand,
		},
		{
			name: "invalid default as from config",
			cfg: &internalsec.Config{
				Enable:    true,
				Auth:      true,
				AppID:     "cli_test_app",
				Brand:     "lark",
				DefaultAs: "bad",
			},
			want: "invalid " + envvars.CliDefaultAs,
		},
		{
			name:     "invalid default as from env",
			cfg:      &internalsec.Config{Enable: true, Auth: true, AppID: "cli_test_app", Brand: "lark"},
			envKey:   envvars.CliDefaultAs,
			envValue: "bad",
			want:     "invalid " + envvars.CliDefaultAs,
		},
		{
			name:     "invalid strict mode from env",
			cfg:      &internalsec.Config{Enable: true, Auth: true, AppID: "cli_test_app", Brand: "lark"},
			envKey:   envvars.CliStrictMode,
			envValue: "bad",
			want:     "invalid " + envvars.CliStrictMode,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			loadSecConfig = func() (*internalsec.Config, error) { return tt.cfg, nil }
			t.Setenv(envvars.CliAppID, "")
			t.Setenv(envvars.CliBrand, "")
			t.Setenv(envvars.CliDefaultAs, "")
			t.Setenv(envvars.CliStrictMode, "")
			if tt.envKey != "" {
				t.Setenv(tt.envKey, tt.envValue)
			}

			acct, err := (&Provider{}).ResolveAccount(context.Background())
			if err == nil {
				t.Fatal("ResolveAccount() error = nil, want block error")
			}
			if acct != nil {
				t.Fatalf("ResolveAccount() = %#v, want nil", acct)
			}
			blockErr, ok := err.(*credential.BlockError)
			if !ok {
				t.Fatalf("ResolveAccount() error = %T, want *credential.BlockError", err)
			}
			if !strings.Contains(blockErr.Reason, tt.want) {
				t.Fatalf("blockErr.Reason = %q, want substring %q", blockErr.Reason, tt.want)
			}
		})
	}
}

// TestProvider_ResolveAccount_FallbackToDiskConfig verifies fallback behavior
// when SEC config omits app identity fields.
func TestProvider_ResolveAccount_FallbackToDiskConfig(t *testing.T) {
	prev := loadSecConfig
	t.Cleanup(func() { loadSecConfig = prev })

	loadSecConfig = func() (*internalsec.Config, error) {
		return &internalsec.Config{Enable: true, Auth: true}, nil
	}

	t.Run("missing config blocks", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
		t.Setenv(envvars.CliAppID, "")
		t.Setenv(envvars.CliBrand, "")
		acct, err := (&Provider{}).ResolveAccount(context.Background())
		if err == nil {
			t.Fatal("ResolveAccount() error = nil, want block error")
		}
		if acct != nil {
			t.Fatalf("ResolveAccount() = %#v, want nil", acct)
		}
		blockErr := err.(*credential.BlockError)
		if !strings.Contains(blockErr.Reason, "no app config is available") {
			t.Fatalf("blockErr.Reason = %q, want missing app config message", blockErr.Reason)
		}
	})

	t.Run("missing active profile blocks", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
		if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
			CurrentApp: "missing",
			Apps: []core.AppConfig{{
				Name:      "default",
				AppId:     "app_from_disk",
				AppSecret: core.PlainSecret("secret"),
				Brand:     core.LarkBrand("lark"),
			}},
		}); err != nil {
			t.Fatalf("SaveMultiAppConfig() error = %v", err)
		}

		acct, err := (&Provider{}).ResolveAccount(context.Background())
		if err == nil {
			t.Fatal("ResolveAccount() error = nil, want block error")
		}
		if acct != nil {
			t.Fatalf("ResolveAccount() = %#v, want nil", acct)
		}
		blockErr := err.(*credential.BlockError)
		if !strings.Contains(blockErr.Reason, "no active profile") {
			t.Fatalf("blockErr.Reason = %q, want no active profile message", blockErr.Reason)
		}
	})

	t.Run("strict mode from disk", func(t *testing.T) {
		cases := []struct {
			name    string
			mode    core.StrictMode
			wantIDs credential.IdentitySupport
		}{
			{name: "bot", mode: core.StrictModeBot, wantIDs: credential.SupportsBot},
			{name: "user", mode: core.StrictModeUser, wantIDs: credential.SupportsUser},
		}

		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				mode := tt.mode
				if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
					Apps: []core.AppConfig{{
						Name:       "default",
						AppId:      "app_from_disk",
						AppSecret:  core.PlainSecret("secret"),
						Brand:      core.LarkBrand("lark"),
						DefaultAs:  core.AsBot,
						StrictMode: &mode,
					}},
				}); err != nil {
					t.Fatalf("SaveMultiAppConfig() error = %v", err)
				}

				acct, err := (&Provider{}).ResolveAccount(context.Background())
				if err != nil {
					t.Fatalf("ResolveAccount() error = %v", err)
				}
				if acct == nil {
					t.Fatal("ResolveAccount() = nil, want account")
				}
				if acct.AppID != "app_from_disk" {
					t.Fatalf("acct.AppID = %q, want app_from_disk", acct.AppID)
				}
				if acct.Brand != credential.Brand("lark") {
					t.Fatalf("acct.Brand = %q, want lark", acct.Brand)
				}
				if acct.DefaultAs != credential.Identity("bot") {
					t.Fatalf("acct.DefaultAs = %q, want bot", acct.DefaultAs)
				}
				if acct.SupportedIdentities != tt.wantIDs {
					t.Fatalf("acct.SupportedIdentities = %d, want %d", acct.SupportedIdentities, tt.wantIDs)
				}
			})
		}
	})
}

// TestProvider_ResolveAccount_StrictModePreservesConfiguredDefaultAs verifies
// cfg.DefaultAs is not overwritten by disk profile default in strict-mode path.
func TestProvider_ResolveAccount_StrictModePreservesConfiguredDefaultAs(t *testing.T) {
	prev := loadSecConfig
	t.Cleanup(func() { loadSecConfig = prev })

	loadSecConfig = func() (*internalsec.Config, error) {
		return &internalsec.Config{
			Enable:     true,
			Auth:       true,
			Brand:      "lark",
			DefaultAs:  "user",
			StrictMode: "bot",
		}, nil
	}

	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliBrand, "")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliStrictMode, "")
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "app_from_disk",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.LarkBrand("lark"),
			DefaultAs: core.AsBot,
		}},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct == nil {
		t.Fatal("ResolveAccount() = nil, want account")
	}
	if acct.DefaultAs != credential.IdentityUser {
		t.Fatalf("acct.DefaultAs = %q, want %q", acct.DefaultAs, credential.IdentityUser)
	}
	if acct.SupportedIdentities != credential.SupportsBot {
		t.Fatalf("acct.SupportedIdentities = %d, want %d (SupportsBot)", acct.SupportedIdentities, credential.SupportsBot)
	}
}

// TestProvider_ResolveToken_ReturnsSentinels verifies placeholder token behavior
// for SEC_AUTH mode.
func TestProvider_ResolveToken_ReturnsSentinels(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliSecEnable, "true")
	t.Setenv(envvars.CliSecAuth, "true")
	t.Setenv(envvars.CliSecProxy, "http://127.0.0.1:3128")
	t.Setenv(envvars.CliSecCA, "")

	p := &Provider{}

	uat, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatalf("ResolveToken(UAT) error = %v", err)
	}
	if uat == nil || uat.Value != internalsec.SentinelUAT || uat.Source != "secplugin" {
		t.Fatalf("ResolveToken(UAT) = %#v, want sentinel UAT token", uat)
	}

	tat, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT})
	if err != nil {
		t.Fatalf("ResolveToken(TAT) error = %v", err)
	}
	if tat == nil || tat.Value != internalsec.SentinelTAT || tat.Source != "secplugin" {
		t.Fatalf("ResolveToken(TAT) = %#v, want sentinel TAT token", tat)
	}

	tok, err := p.ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenType("other")})
	if err != nil {
		t.Fatalf("ResolveToken(other) error = %v", err)
	}
	if tok != nil {
		t.Fatalf("ResolveToken(other) = %#v, want nil", tok)
	}
}
