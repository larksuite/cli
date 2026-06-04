// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// stubUsersConfig builds a temp config dir + multi-app config with `target`
// profile populated from users (insertion order preserved), and returns a
// Factory wired to that profile.
func stubUsersConfig(t *testing.T, currentUser string, users []core.AppUser) (*cmdutil.Factory, *core.MultiAppConfig) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	multi := &core.MultiAppConfig{
		CurrentApp: "target",
		Apps: []core.AppConfig{{
			Name:        "target",
			AppId:       "app-target",
			Brand:       core.BrandFeishu,
			CurrentUser: currentUser,
			Users:       users,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "target", AppID: "app-target", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	f.Invocation = cmdutil.InvocationContext{Profile: "target"}
	return f, multi
}

func TestAuthUsersListRun_ActiveMarker(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	f, _ := stubUsersConfig(t, "ou_bob", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice", FirstAuthAt: &now, LastUsed: &now, LastScopes: "im:message:send"},
		{UserOpenId: "ou_bob", UserName: "Bob", FirstAuthAt: &now, LastUsed: &now},
	})

	if err := authUsersListRun(&UsersListOptions{Factory: f}); err != nil {
		t.Fatalf("authUsersListRun: %v", err)
	}
	stdout := f.IOStreams.Out.(interface{ String() string }).String()

	var got []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode JSON: %v\noutput: %s", err, stdout)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0]["userOpenId"] != "ou_alice" || got[1]["userOpenId"] != "ou_bob" {
		t.Errorf("order = %v, want [alice,bob]", got)
	}
	if got[0]["active"] != false || got[1]["active"] != true {
		t.Errorf("active markers wrong: %v", got)
	}
	if got[0]["lastScopes"] != "im:message:send" {
		t.Errorf("alice.lastScopes = %v, want im:message:send", got[0]["lastScopes"])
	}
	if got[0]["firstAuthAt"] == nil {
		t.Error("firstAuthAt missing")
	}
}

// TestAuthUsersListRun_EmptyProfile: hint to stderr, no error (matches
// `auth list` empty-state contract).
func TestAuthUsersListRun_EmptyProfile(t *testing.T) {
	f, _ := stubUsersConfig(t, "", nil)
	if err := authUsersListRun(&UsersListOptions{Factory: f}); err != nil {
		t.Fatalf("authUsersListRun: %v", err)
	}
	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()
	if !strings.Contains(stderr, "No users in this profile") {
		t.Errorf("stderr missing empty-state hint: %q", stderr)
	}
}

// TestAuthUsersListRun_OverrideMarksRequestedUser: --user override wins over
// AppConfig.CurrentUser when picking the active row.
func TestAuthUsersListRun_OverrideMarksRequestedUser(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
		{UserOpenId: "ou_bob", UserName: "Bob"},
	})
	f.Invocation.UserOpenId = "ou_bob"

	if err := authUsersListRun(&UsersListOptions{Factory: f}); err != nil {
		t.Fatalf("authUsersListRun: %v", err)
	}
	stdout := f.IOStreams.Out.(interface{ String() string }).String()
	var got []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got[1]["active"] != true {
		t.Errorf("--user ou_bob should mark Bob active, got %v", got)
	}
}

func TestAuthUsersUseRun_SwitchesActive(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
		{UserOpenId: "ou_bob", UserName: "Bob"},
	})
	if err := authUsersUseRun(&UsersUseOptions{Factory: f, Target: "ou_bob"}); err != nil {
		t.Fatalf("authUsersUseRun: %v", err)
	}
	saved, _ := core.LoadMultiAppConfig()
	if saved.Apps[0].CurrentUser != "ou_bob" {
		t.Errorf("CurrentUser = %q, want ou_bob", saved.Apps[0].CurrentUser)
	}
}

// TestAuthUsersUseRun_ResolvesByName: open_id-first then UserName fallback.
func TestAuthUsersUseRun_ResolvesByName(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
		{UserOpenId: "ou_bob", UserName: "Bob"},
	})
	if err := authUsersUseRun(&UsersUseOptions{Factory: f, Target: "Bob"}); err != nil {
		t.Fatalf("authUsersUseRun: %v", err)
	}
	saved, _ := core.LoadMultiAppConfig()
	if saved.Apps[0].CurrentUser != "ou_bob" {
		t.Errorf("CurrentUser = %q, want ou_bob (resolved by name)", saved.Apps[0].CurrentUser)
	}
}

func TestAuthUsersUseRun_NoOpIfAlreadyActive(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
	})
	if err := authUsersUseRun(&UsersUseOptions{Factory: f, Target: "ou_alice"}); err != nil {
		t.Fatalf("authUsersUseRun: %v", err)
	}
	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()
	if !strings.Contains(stderr, "Already active") {
		t.Errorf("stderr should mention 'Already active': %q", stderr)
	}
}

func TestAuthUsersUseRun_MissTypedError(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
	})
	err := authUsersUseRun(&UsersUseOptions{Factory: f, Target: "ou_ghost"})
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) || cfgErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected ConfigError(InvalidArgument), got %T %v", err, err)
	}
	if !strings.Contains(cfgErr.Hint, "Alice") {
		t.Errorf("hint should list available users: %q", cfgErr.Hint)
	}
}

func TestAuthUsersUseRun_EmptyTarget(t *testing.T) {
	f, _ := stubUsersConfig(t, "", []core.AppUser{{UserOpenId: "ou_alice"}})
	err := authUsersUseRun(&UsersUseOptions{Factory: f, Target: "  "})
	if err == nil {
		t.Fatal("expected error for whitespace-only target")
	}
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) || vErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("expected ValidationError(InvalidArgument), got %T %v", err, err)
	}
}

// TestAuthUsersLogoutRun_RemovesUserAndToken: keychain token, config row,
// sidecar profile, and index entry are all wiped.
func TestAuthUsersLogoutRun_RemovesUserAndToken(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
		{UserOpenId: "ou_bob", UserName: "Bob"},
	})
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "app-target", UserOpenId: "ou_bob", AccessToken: "tok",
	}); err != nil {
		t.Fatalf("SetStoredToken: %v", err)
	}

	if err := authUsersLogoutRun(&UsersLogoutOptions{Factory: f, Target: "ou_bob"}); err != nil {
		t.Fatalf("authUsersLogoutRun: %v", err)
	}
	saved, _ := core.LoadMultiAppConfig()
	if len(saved.Apps[0].Users) != 1 || saved.Apps[0].Users[0].UserOpenId != "ou_alice" {
		t.Errorf("Users = %#v, want only Alice remaining", saved.Apps[0].Users)
	}
	if saved.Apps[0].CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser unchanged because we logged out a non-active user, got %q", saved.Apps[0].CurrentUser)
	}
	if got := larkauth.GetStoredToken("app-target", "ou_bob"); got != nil {
		t.Errorf("Bob's token still in keychain: %#v", got)
	}
}

// TestAuthUsersLogoutRun_ClearsCurrentUserIfActive: logging out the active
// user clears CurrentUser; no auto-switch to another row.
func TestAuthUsersLogoutRun_ClearsCurrentUserIfActive(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
		{UserOpenId: "ou_bob", UserName: "Bob"},
	})
	if err := authUsersLogoutRun(&UsersLogoutOptions{Factory: f, Target: "ou_alice"}); err != nil {
		t.Fatalf("authUsersLogoutRun: %v", err)
	}
	saved, _ := core.LoadMultiAppConfig()
	if saved.Apps[0].CurrentUser != "" {
		t.Errorf("CurrentUser = %q, want empty (no auto-switch)", saved.Apps[0].CurrentUser)
	}
	if len(saved.Apps[0].Users) != 1 || saved.Apps[0].Users[0].UserOpenId != "ou_bob" {
		t.Errorf("Users = %#v, want only Bob", saved.Apps[0].Users)
	}
}

func TestAuthUsersLogoutRun_MissTypedError(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}})
	err := authUsersLogoutRun(&UsersLogoutOptions{Factory: f, Target: "ou_ghost"})
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) || cfgErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected ConfigError(InvalidArgument), got %T %v", err, err)
	}
}

func TestAuthUsersLogoutRun_EmptyTarget(t *testing.T) {
	f, _ := stubUsersConfig(t, "", []core.AppUser{{UserOpenId: "ou_alice"}})
	err := authUsersLogoutRun(&UsersLogoutOptions{Factory: f, Target: ""})
	if err == nil {
		t.Fatal("expected error for empty target")
	}
}
