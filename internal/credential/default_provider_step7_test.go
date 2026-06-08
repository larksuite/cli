// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

type stubKC struct{}

func (stubKC) Get(service, account string) (string, error) { return "", keychain.ErrNotFound }
func (stubKC) Set(service, account, value string) error    { return nil }
func (stubKC) Remove(service, account string) error        { return nil }

// writeMulti persists cfg to a temp config dir scoped to t. Exercises the
// real core.LoadMultiAppConfig path rather than mocking it.
func writeMulti(t *testing.T, cfg *core.MultiAppConfig) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := core.SaveMultiAppConfig(cfg); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}
}

func TestResolveAccount_UserOverride_FlowsThroughResolveConfigFromMulti(t *testing.T) {
	writeMulti(t, &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
			Users: []core.AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
				{UserOpenId: "ou_b", UserName: "Bob"},
			},
		}},
	})

	p := NewDefaultAccountProvider(func() keychain.KeychainAccess { return stubKC{} }, "", "ou_b", "flag")
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount: %v", err)
	}
	if acct.UserOpenId != "ou_b" {
		t.Errorf("UserOpenId = %q, want ou_b (override should beat Users[0])", acct.UserOpenId)
	}
}

// Empty override must fall through to Users[0] when CurrentUser is unset.
func TestResolveAccount_UserOverrideEmpty_Regression(t *testing.T) {
	writeMulti(t, &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_a", UserName: "Alice"}},
		}},
	})

	p := NewDefaultAccountProvider(func() keychain.KeychainAccess { return stubKC{} }, "", "", "")
	acct, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount: %v", err)
	}
	if acct.UserOpenId != "ou_a" {
		t.Errorf("empty override should fall through to Users[0], got %q", acct.UserOpenId)
	}
}

func TestResolveAccount_UserOverrideMiss_TypedError(t *testing.T) {
	writeMulti(t, &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_a", UserName: "Alice"}},
		}},
	})

	p := NewDefaultAccountProvider(func() keychain.KeychainAccess { return stubKC{} }, "", "ou_ghost", "")
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *core.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Code != 3 || cfgErr.Type != "config" {
		t.Errorf("err shape: code=%d type=%q, want 3/config", cfgErr.Code, cfgErr.Type)
	}
	if !strings.Contains(cfgErr.Message, "ou_ghost") {
		t.Errorf("message missing requested user: %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "Alice") {
		t.Errorf("hint should list available users: %q", cfgErr.Hint)
	}
	if strings.Contains(cfgErr.Hint, "LARKSUITE_CLI_OPEN_ID") {
		t.Errorf("hint should not mention env var when source is empty: %q", cfgErr.Hint)
	}
}

// source="env" must add the unset-env remediation suffix to disambiguate
// stale shell env from a typo'd flag.
func TestResolveAccount_UserOverrideMiss_FromEnv_HasEnvHint(t *testing.T) {
	writeMulti(t, &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_a", UserName: "Alice"}},
		}},
	})

	p := NewDefaultAccountProvider(func() keychain.KeychainAccess { return stubKC{} }, "", "ou_ghost", "env")
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *core.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(cfgErr.Hint, "LARKSUITE_CLI_OPEN_ID") {
		t.Errorf("source=env hint should mention LARKSUITE_CLI_OPEN_ID, got: %q", cfgErr.Hint)
	}
	if !strings.Contains(cfgErr.Hint, "unset") {
		t.Errorf("source=env hint should suggest unsetting, got: %q", cfgErr.Hint)
	}
}

// source="flag" must not stamp the env suffix; resolver hint already names --user.
func TestResolveAccount_UserOverrideMiss_FromFlag_NoEnvHint(t *testing.T) {
	writeMulti(t, &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_a", UserName: "Alice"}},
		}},
	})

	p := NewDefaultAccountProvider(func() keychain.KeychainAccess { return stubKC{} }, "", "ou_ghost", "flag")
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *core.ConfigError, got %T: %v", err, err)
	}
	if strings.Contains(cfgErr.Hint, "LARKSUITE_CLI_OPEN_ID") {
		t.Errorf("source=flag hint should NOT mention LARKSUITE_CLI_OPEN_ID, got: %q", cfgErr.Hint)
	}
	if !strings.Contains(cfgErr.Hint, "auth login") {
		t.Errorf("hint should still carry resolver's recovery copy: %q", cfgErr.Hint)
	}
}

// Profile-miss messages don't contain "user", so the user-env decoration
// must not be stamped onto them.
func TestResolveAccount_ProfileMiss_NotDecoratedWithEnvHint(t *testing.T) {
	writeMulti(t, &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name: "alpha", AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
		}},
	})

	p := NewDefaultAccountProvider(func() keychain.KeychainAccess { return stubKC{} }, "ghostprofile", "ou_alice", "env")
	_, err := p.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected profile-miss error, got nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *core.ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(cfgErr.Message, "profile") {
		t.Errorf("expected profile-miss message, got: %q", cfgErr.Message)
	}
	if strings.Contains(cfgErr.Hint, "LARKSUITE_CLI_OPEN_ID") {
		t.Errorf("profile-miss hint should not be stamped with user-env suffix: %q", cfgErr.Hint)
	}
}
