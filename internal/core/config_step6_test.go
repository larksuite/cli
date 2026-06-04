// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"strings"
	"testing"
)

// Resolver fallback order:
//   1. userOverride non-empty: FindUser(userOverride); miss = error
//   2. AppConfig.CurrentUser non-empty: FindUser(CurrentUser); miss = drift error
//   3. else len(Users) > 0: pick &Users[0] (legacy single-user path)
//   4. else: cfg.UserOpenId stays empty (RequireAuth* surfaces "not logged in")

// Legacy contract: one user, no CurrentUser, no override must resolve from Users[0].
func TestResolveConfigFromMulti_LegacyUsersZeroPathUnchanged(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
			},
		}},
	}
	cfg, err := ResolveConfigFromMulti(raw, nil, "", "")
	if err != nil {
		t.Fatalf("legacy path errored: %v", err)
	}
	if cfg.UserOpenId != "ou_a" || cfg.UserName != "Alice" {
		t.Errorf("Users[0] not resolved: cfg=%+v", cfg)
	}
}

// Empty Users[] does not error here; RequireAuth* wraps that as "not logged in".
func TestResolveConfigFromMulti_EmptyUsersStaysEmpty(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{},
		}},
	}
	cfg, err := ResolveConfigFromMulti(raw, nil, "", "")
	if err != nil {
		t.Fatalf("empty Users[]: resolver errored unexpectedly: %v", err)
	}
	if cfg.UserOpenId != "" || cfg.UserName != "" {
		t.Errorf("empty Users[] should leave user fields empty, got: %+v", cfg)
	}
}

// ─── Rung 2: AppConfig.CurrentUser ──────────────────────────────────────

func TestResolveConfigFromMulti_HonoursAppConfigCurrentUser(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			CurrentUser: "ou_b",
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
				{UserOpenId: "ou_b", UserName: "Bob"},
			},
		}},
	}
	cfg, err := ResolveConfigFromMulti(raw, nil, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.UserOpenId != "ou_b" || cfg.UserName != "Bob" {
		t.Errorf("CurrentUser not honoured: got %+v", cfg)
	}
}

// Stale CurrentUser must NOT silently fall back to Users[0] — that would
// dispatch as the wrong human. Drift error must include both --user and
// `auth users use` remediation paths.
func TestResolveConfigFromMulti_StaleCurrentUser_DoesNotFallbackToUsers0(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			CurrentUser: "ou_ghost",
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
			},
		}},
	}
	_, err := ResolveConfigFromMulti(raw, nil, "", "")
	if err == nil {
		t.Fatal("expected error for stale CurrentUser, got nil")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Code != 3 || cfgErr.Type != "config" {
		t.Errorf("err shape: code=%d type=%q, want 3/config", cfgErr.Code, cfgErr.Type)
	}
	if !strings.Contains(cfgErr.Message, "current user") || !strings.Contains(cfgErr.Message, "ou_ghost") {
		t.Errorf("message missing key terms: %q", cfgErr.Message)
	}
	// Hint must mention both the one-shot --user override and the
	// permanent `auth users use` / `auth login` recovery paths.
	if !strings.Contains(cfgErr.Hint, "--user") {
		t.Errorf("hint missing --user remediation: %q", cfgErr.Hint)
	}
	if !strings.Contains(cfgErr.Hint, "auth login") {
		t.Errorf("hint missing auth login remediation: %q", cfgErr.Hint)
	}
	if !strings.Contains(cfgErr.Hint, "Alice") {
		t.Errorf("hint should list available users: %q", cfgErr.Hint)
	}
}

// ─── Rung 1: explicit userOverride ──────────────────────────────────────

func TestResolveConfigFromMulti_UserOverrideTakesPrecedenceOverCurrentUser(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			CurrentUser: "ou_a",
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
				{UserOpenId: "ou_b", UserName: "Bob"},
			},
		}},
	}
	cfg, err := ResolveConfigFromMulti(raw, nil, "", "ou_b")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.UserOpenId != "ou_b" || cfg.UserName != "Bob" {
		t.Errorf("override not respected: got %+v", cfg)
	}
}

// OpenId match wins over name match: a UserName equal to another user's
// OpenId must NOT shadow the real OpenId owner.
func TestResolveConfigFromMulti_UserOverrideMatchesByOpenId(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{
				// User a's UserName equals user b's OpenId — pathological but legal.
				{UserOpenId: "ou_a", UserName: "ou_b"},
				{UserOpenId: "ou_b", UserName: "Bob"},
			},
		}},
	}
	cfg, err := ResolveConfigFromMulti(raw, nil, "", "ou_b")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.UserOpenId != "ou_b" || cfg.UserName != "Bob" {
		t.Errorf("name-impostor matched instead of OpenId: got %+v", cfg)
	}
}

// Override falls back to UserName when OpenId match fails — so operators
// can pass --user "Alice" without copying ou_xxx.
func TestResolveConfigFromMulti_UserOverrideMatchesByName(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
				{UserOpenId: "ou_b", UserName: "Bob"},
			},
		}},
	}
	cfg, err := ResolveConfigFromMulti(raw, nil, "", "Alice")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.UserOpenId != "ou_a" {
		t.Errorf("name match failed: got %+v", cfg)
	}
}

// Override miss → ConfigError{Code:3,Type:"config"} (matches existing
// renderer); hint must list available users + suggest auth login.
func TestResolveConfigFromMulti_UserOverrideUnknown_TypedErrorWithHint(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
			},
		}},
	}
	_, err := ResolveConfigFromMulti(raw, nil, "", "ou_z")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Code != 3 || cfgErr.Type != "config" {
		t.Errorf("err shape: code=%d type=%q, want 3/config", cfgErr.Code, cfgErr.Type)
	}
	if !strings.Contains(cfgErr.Message, "ou_z") || !strings.Contains(cfgErr.Message, "not found") {
		t.Errorf("message missing key terms: %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "Alice") {
		t.Errorf("hint should list available users: %q", cfgErr.Hint)
	}
	if !strings.Contains(cfgErr.Hint, "auth login") {
		t.Errorf("hint should suggest auth login: %q", cfgErr.Hint)
	}
	// Drift hint copy MUST NOT appear here — explicit override miss has
	// different remediation than the CurrentUser-stale case.
	if strings.Contains(cfgErr.Hint, "config.json was hand-edited") {
		t.Error("hint should not include drift text for explicit override miss")
	}
}

// Empty users + --user must still render hint cleanly with "(none)".
func TestResolveConfigFromMulti_UserOverrideUnknown_EmptyUsers_HintShowsNone(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{},
		}},
	}
	_, err := ResolveConfigFromMulti(raw, nil, "", "ou_z")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cfgErr *ConfigError
	errors.As(err, &cfgErr)
	if !strings.Contains(cfgErr.Hint, "(none)") {
		t.Errorf("hint should show (none) for empty users, got: %q", cfgErr.Hint)
	}
}

// userOverride="" must fall through to rungs 2/3, not error — every
// existing call site passes "" today.
func TestResolveConfigFromMulti_EmptyUserOverride_TreatedAsUnset(t *testing.T) {
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
			},
		}},
	}
	cfg, err := ResolveConfigFromMulti(raw, nil, "", "")
	if err != nil {
		t.Fatalf("empty override should not error: %v", err)
	}
	if cfg.UserOpenId != "ou_a" {
		t.Errorf("empty override should fall through to Users[0], got: %+v", cfg)
	}
}

// Resolver is env-agnostic: LARKSUITE_CLI_OPEN_ID does NOT inject when
// userOverride="". Bootstrap plumbs env→string before calling resolver,
// mirroring --profile (see TestResolveConfigFromMulti_DoesNotUseEnvProfileFallback).
func TestResolveConfigFromMulti_DoesNotReadEnvForUserOverride(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_OPEN_ID", "ou_b")
	raw := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
				{UserOpenId: "ou_b", UserName: "Bob"},
			},
		}},
	}
	cfg, err := ResolveConfigFromMulti(raw, nil, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.UserOpenId != "ou_a" {
		t.Errorf("env should be ignored; expected Users[0]=ou_a, got %+v", cfg)
	}
}

// ─── RequireAuth* wrappers ──────────────────────────────────────────────

// Drift error from ResolveConfigFromMulti must propagate untransformed
// through the Auth wrapper so operators see the same recovery hint.
func TestRequireAuthForProfileAndUser_StaleCurrentUser_SurfacesConfigError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cfg := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			CurrentUser: "ou_ghost",
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
			},
		}},
	}
	if err := SaveMultiAppConfig(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := RequireAuthForProfileAndUser(nil, "", "")
	if err == nil {
		t.Fatal("expected drift ConfigError, got nil")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Type != "config" {
		t.Errorf("type=%q, want config (drift propagated, not converted to auth)", cfgErr.Type)
	}
	if !strings.Contains(cfgErr.Message, "ou_ghost") {
		t.Errorf("drift message lost: %q", cfgErr.Message)
	}
}

// Legacy zero-arg helper must return same CliConfig as *AndUser sibling
// with userOverride="" — locks the thin-forwarder contract.
func TestRequireConfigForProfile_LegacyShape_StillWorks(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cfg := &MultiAppConfig{
		Apps: []AppConfig{{
			AppId: "cli_x", AppSecret: PlainSecret("s"), Brand: BrandFeishu,
			Users: []AppUser{
				{UserOpenId: "ou_a", UserName: "Alice"},
			},
		}},
	}
	if err := SaveMultiAppConfig(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	legacy, err := RequireConfigForProfile(nil, "")
	if err != nil {
		t.Fatalf("legacy helper: %v", err)
	}
	via, err := RequireConfigForProfileAndUser(nil, "", "")
	if err != nil {
		t.Fatalf("AndUser helper: %v", err)
	}
	if legacy.UserOpenId != via.UserOpenId || legacy.AppID != via.AppID {
		t.Errorf("forwarder drift: legacy=%+v vs sibling=%+v", legacy, via)
	}
}

// ─── FindUser / FindUserIndex / UserNames helpers ───────────────────────

// OpenId match wins over name match on collision.
func TestFindUser_OpenIdTakesPrecedenceOverNameOnConflict(t *testing.T) {
	app := &AppConfig{
		Users: []AppUser{
			{UserOpenId: "ou_a", UserName: "ou_b"}, // name impostor
			{UserOpenId: "ou_b", UserName: "Bob"},
		},
	}
	got := app.FindUser("ou_b")
	if got == nil {
		t.Fatal("FindUser returned nil for valid OpenId")
	}
	if got.UserOpenId != "ou_b" || got.UserName != "Bob" {
		t.Errorf("name impostor matched: got %+v", got)
	}
}

// Empty input must NOT match an AppUser with empty UserName (legitimate
// for service accounts).
func TestFindUser_EmptyInputReturnsNil(t *testing.T) {
	app := &AppConfig{
		Users: []AppUser{
			{UserOpenId: "ou_a", UserName: ""},
		},
	}
	if got := app.FindUser(""); got != nil {
		t.Errorf("empty input matched %+v", got)
	}
}

func TestFindUser_NameMatchFallback(t *testing.T) {
	app := &AppConfig{
		Users: []AppUser{
			{UserOpenId: "ou_a", UserName: "Alice"},
			{UserOpenId: "ou_b", UserName: "Bob"},
		},
	}
	got := app.FindUser("Bob")
	if got == nil || got.UserOpenId != "ou_b" {
		t.Errorf("name fallback failed: got %+v", got)
	}
}

func TestFindUser_NotFoundReturnsNil(t *testing.T) {
	app := &AppConfig{
		Users: []AppUser{
			{UserOpenId: "ou_a", UserName: "Alice"},
		},
	}
	if got := app.FindUser("ou_z"); got != nil {
		t.Errorf("expected nil for missing user, got %+v", got)
	}
}

// Index-returning sibling of FindUser; -1 means not-found, mirroring FindAppIndex.
func TestFindUserIndex(t *testing.T) {
	app := &AppConfig{
		Users: []AppUser{
			{UserOpenId: "ou_a", UserName: "Alice"},
			{UserOpenId: "ou_b", UserName: "Bob"},
		},
	}
	if i := app.FindUserIndex("ou_b"); i != 1 {
		t.Errorf("FindUserIndex(ou_b) = %d, want 1", i)
	}
	if i := app.FindUserIndex("Alice"); i != 0 {
		t.Errorf("FindUserIndex(Alice) = %d, want 0", i)
	}
	if i := app.FindUserIndex("ou_z"); i != -1 {
		t.Errorf("FindUserIndex(missing) = %d, want -1", i)
	}
	if i := app.FindUserIndex(""); i != -1 {
		t.Errorf("FindUserIndex(empty) = %d, want -1", i)
	}
}

// Rendering used by error hints and `auth users list`; stable across
// releases for operators scripting around the output.
func TestUserNames_FormatStable(t *testing.T) {
	app := &AppConfig{
		Users: []AppUser{
			{UserOpenId: "ou_a", UserName: "Alice"},
			{UserOpenId: "ou_serviceaccount", UserName: ""},
		},
	}
	names := app.UserNames()
	if len(names) != 2 {
		t.Fatalf("UserNames() len = %d, want 2", len(names))
	}
	if names[0] != "Alice (ou_a)" {
		t.Errorf("names[0] = %q, want %q", names[0], "Alice (ou_a)")
	}
	if names[1] != "ou_serviceaccount" {
		t.Errorf("names[1] = %q, want %q (no name shows OpenId only)", names[1], "ou_serviceaccount")
	}
}

// Long OpenIds truncate to 12 chars + "…" so hints stay terminal-readable.
func TestFormatUserDisplay_TruncatesLongOpenIds(t *testing.T) {
	users := []AppUser{
		{UserOpenId: "ou_aaaaaaaaaaaaaaaaaaaaaa", UserName: "Alice"},
	}
	got := formatUserDisplay(users)
	const wantSubstr = "Alice (ou_aaaaaaaaa…)"
	if !strings.Contains(got, wantSubstr) {
		t.Errorf("expected truncation %q, got: %q", wantSubstr, got)
	}
}

// "(none)" branch used in error hints.
func TestFormatUserDisplay_EmptyReturnsNone(t *testing.T) {
	if got := formatUserDisplay([]AppUser{}); got != "(none)" {
		t.Errorf("empty users render = %q, want (none)", got)
	}
}
