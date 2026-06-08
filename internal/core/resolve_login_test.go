// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/internal/keychain"
)

// Regression: ResolveConfigFromMulti enforces a strict user-rung selector.
// That is correct for "use this user to make a call", but `auth login
// --user ou_new` is the path that ADDS a new user to the profile —
// strict resolution there is unreachable by definition. Pre-fix,
// `auth login --user ou_new_user` errored out before the device flow
// even started:
//
//	Error: user "ou_new_user" not found in profile "prod"
//	  available users in this profile: alice (ou_alice)
//
// The new-user login path was structurally unreachable.
//
// ResolveProfileConfigForLogin is the profile-rung-only resolver
// `auth login` now uses. It must return without error when the user
// override names a brand-new open_id, and must STILL surface profile-
// rung errors verbatim (typed *ConfigError with RungProfile) so
// profile typos don't get mis-routed to "not configured".

type stubKCForLogin struct{}

func (stubKCForLogin) Get(service, account string) (string, error) { return "", nil }
func (stubKCForLogin) Set(service, account, value string) error    { return nil }
func (stubKCForLogin) Remove(service, account string) error        { return nil }

func TestResolveProfileConfigForLogin_UnknownUser_DoesNotError(t *testing.T) {
	multi := &MultiAppConfig{
		CurrentApp: "prod",
		Apps: []AppConfig{{
			Name:        "prod",
			AppId:       "cli_prod",
			AppSecret:   PlainSecret("s"),
			Brand:       BrandFeishu,
			CurrentUser: "ou_alice",
			Users: []AppUser{
				{UserOpenId: "ou_alice", UserName: "Alice"},
			},
		}},
	}

	// Unknown user override — would trip ResolveConfigFromMulti's user-
	// rung strict check. ResolveProfileConfigForLogin must skip that
	// rung entirely.
	cfg, err := ResolveProfileConfigForLogin(multi, keychain.KeychainAccess(stubKCForLogin{}), "")
	if err != nil {
		t.Fatalf("ResolveProfileConfigForLogin: %v", err)
	}
	if cfg.AppID != "cli_prod" {
		t.Errorf("AppID = %q, want cli_prod", cfg.AppID)
	}
	if cfg.UserOpenId != "" {
		t.Errorf("UserOpenId = %q, want empty (caller resolves post-auth)", cfg.UserOpenId)
	}
	if cfg.UserName != "" {
		t.Errorf("UserName = %q, want empty (caller resolves post-auth)", cfg.UserName)
	}

	// Sibling proof: ResolveConfigFromMulti with the same unknown user
	// MUST still error — the strict-resolution contract is unchanged
	// for the non-login path.
	if _, err := ResolveConfigFromMulti(multi, keychain.KeychainAccess(stubKCForLogin{}), "", "ou_new_user"); err == nil {
		t.Errorf("ResolveConfigFromMulti must still error for unknown user override; got nil")
	}
}

// Profile-rung errors must still pass through with the right Rung
// tag — `auth login --profile=ghost` shouldn't be silently swallowed.
func TestResolveProfileConfigForLogin_UnknownProfile_ReturnsRungProfile(t *testing.T) {
	multi := &MultiAppConfig{
		Apps: []AppConfig{{
			Name:      "prod",
			AppId:     "cli_prod",
			AppSecret: PlainSecret("s"),
			Brand:     BrandFeishu,
		}},
	}

	_, err := ResolveProfileConfigForLogin(multi, keychain.KeychainAccess(stubKCForLogin{}), "ghost")
	if err == nil {
		t.Fatalf("expected profile-not-found error for --profile=ghost")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("err type = %T, want *core.ConfigError", err)
	}
	if cfgErr.Rung != RungProfile {
		t.Errorf("Rung = %q, want RungProfile", cfgErr.Rung)
	}
}

// When the profile has no current user and no users at all (a fresh
// `config init`-only state), login must still resolve — that's the
// normal "first login on this profile" case.
func TestResolveProfileConfigForLogin_EmptyUsers_DoesNotError(t *testing.T) {
	multi := &MultiAppConfig{
		Apps: []AppConfig{{
			Name:      "prod",
			AppId:     "cli_prod",
			AppSecret: PlainSecret("s"),
			Brand:     BrandFeishu,
			Users:     nil,
		}},
	}
	cfg, err := ResolveProfileConfigForLogin(multi, keychain.KeychainAccess(stubKCForLogin{}), "")
	if err != nil {
		t.Fatalf("ResolveProfileConfigForLogin: %v", err)
	}
	if cfg.AppID != "cli_prod" {
		t.Errorf("AppID = %q, want cli_prod", cfg.AppID)
	}
}
