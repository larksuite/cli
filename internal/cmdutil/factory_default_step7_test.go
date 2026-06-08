// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// End-to-end checks that InvocationContext.UserOpenId/UserSource flow through
// NewDefault → credentialDeps → DefaultAccountProvider.

func TestNewDefault_InvocationUserOpenIdSelectsRequestedUser(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv(envvars.CliOpenID, "") // env must not participate at this layer
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
			Users: []core.AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
				{UserOpenId: "ou_b", UserName: "Bob"},
			},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f := NewDefault(nil, InvocationContext{UserOpenId: "ou_b", UserSource: "flag"})
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if cfg.UserOpenId != "ou_b" {
		t.Errorf("UserOpenId = %q, want ou_b (override should beat Users[0])", cfg.UserOpenId)
	}
	if cfg.UserName != "Bob" {
		t.Errorf("UserName = %q, want Bob", cfg.UserName)
	}
}

// Env-sourced override miss must surface the LARKSUITE_CLI_OPEN_ID hint.
func TestNewDefault_UserOverrideMissProducesEnvHintWhenSourceEnv(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv(envvars.CliOpenID, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_a", UserName: "Alice"}},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f := NewDefault(nil, InvocationContext{UserOpenId: "ou_ghost", UserSource: "env"})
	_, err := f.Config()
	if err == nil {
		t.Fatal("expected user-miss error, got nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *core.ConfigError, got %T", err)
	}
	if !strings.Contains(cfgErr.Hint, "LARKSUITE_CLI_OPEN_ID") {
		t.Errorf("env-sourced miss hint should mention env var, got: %q", cfgErr.Hint)
	}
}

// Zero-value InvocationContext must keep legacy single-user installs working.
func TestNewDefault_EmptyUserOpenIdPreservesLegacyBehaviour(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv(envvars.CliOpenID, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_a", UserName: "Alice"}},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f := NewDefault(nil, InvocationContext{})
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if cfg.UserOpenId != "ou_a" {
		t.Errorf("UserOpenId = %q, want ou_a (legacy single-user fallthrough)", cfg.UserOpenId)
	}
}
