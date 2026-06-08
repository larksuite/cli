// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func stubLoginConfigStep8(t *testing.T, multi *core.MultiAppConfig) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}
}

// stubLoginHTTP registers happy-path device-flow + token + user-info stubs;
// callers can override by re-registering the same URL after this call.
func stubLoginHTTP(t *testing.T, reg *httpmock.Registry, openId, name string) {
	t.Helper()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  0,
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthTokenV2,
		Body: map[string]interface{}{
			"access_token":             "user-access-token",
			"refresh_token":            "refresh-token",
			"expires_in":               7200,
			"refresh_token_expires_in": 604800,
			"scope":                    "im:message:send offline_access",
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    larkauth.PathUserInfoV1,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"open_id":  openId,
				"union_id": "uid_" + openId,
				"name":     name,
			},
		},
	})
}

// Different-user login appends rather than replacing — guards against the
// legacy REPLACE semantics where Bob's login would wipe Alice.
func TestAuthLoginRun_Step8_UpsertNewUserAppendsRow(t *testing.T) {
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	})
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})
	stubLoginHTTP(t, reg, "ou_bob", "Bob")

	f.Invocation = cmdutil.InvocationContext{Profile: "default", UserOpenId: "ou_bob", UserSource: "flag"}

	if err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send",
	}); err != nil {
		t.Fatalf("authLoginRun: %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	users := saved.Apps[0].Users
	if len(users) != 2 {
		t.Fatalf("Users len = %d, want 2 (Alice preserved + Bob appended); got %#v", len(users), users)
	}
	if users[0].UserOpenId != "ou_alice" || users[1].UserOpenId != "ou_bob" {
		t.Errorf("Users order = [%q,%q], want [ou_alice,ou_bob]", users[0].UserOpenId, users[1].UserOpenId)
	}
	if saved.Apps[0].CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser = %q, want ou_alice (must NOT silently switch on different-user login)", saved.Apps[0].CurrentUser)
	}
	// Regression guard: deleted destructive cleanup loop must not touch Alice's slot.
	if got := larkauth.GetStoredToken("cli_test", "ou_alice"); got != nil {
		t.Errorf("Alice's slot was somehow populated: %#v", got)
	}
	if got := larkauth.GetStoredToken("cli_test", "ou_bob"); got == nil {
		t.Fatal("Bob's token slot is empty after his login")
	}
}

// --user disagrees with upstream open_id: must abort before SetStoredToken
// with a flag-attributed SubtypeInvalidArgument error.
func TestAuthLoginRun_Step8_HolderMismatch_FlagPath(t *testing.T) {
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{Name: "default", AppId: "cli_test"}},
	})
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stubLoginHTTP(t, reg, "ou_actually_bob", "Bob")

	f.Invocation = cmdutil.InvocationContext{Profile: "default", UserOpenId: "ou_alice", UserSource: "flag"}

	err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send",
	})
	if err == nil {
		t.Fatal("expected holder-mismatch error, got nil")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want SubtypeInvalidArgument", cfgErr.Subtype)
	}
	if !strings.Contains(cfgErr.Hint, "--user") {
		t.Errorf("flag-source hint should mention --user: %q", cfgErr.Hint)
	}
	// Pre-write abort: nothing persisted for the upstream user.
	if got := larkauth.GetStoredToken("cli_test", "ou_actually_bob"); got != nil {
		t.Errorf("Bob's token was stored despite holder mismatch: %#v", got)
	}
	saved, _ := core.LoadMultiAppConfig()
	for _, u := range saved.Apps[0].Users {
		if u.UserOpenId == "ou_actually_bob" {
			t.Errorf("config grew an ou_actually_bob row despite mismatch: %#v", u)
		}
	}
}

// Holder mismatch + downstream sync failure must leave the prior token slot
// intact. We exercise restoreStoredToken's contract directly via the unit
// tests below (synthesizing a mid-flight failure here is not feasible cleanly).
func TestRestoreStoredToken_PriorPresent_RestoresPrior(t *testing.T) {
	keyring.MockInit()
	prior := &larkauth.StoredUAToken{
		AppId: "cli_test", UserOpenId: "ou_alice",
		AccessToken: "prior-tok", RefreshToken: "prior-r",
		ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
	}
	// Caller's "set then fail" sequence: write the new token, then rollback.
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_test", UserOpenId: "ou_alice", AccessToken: "new-tok",
	}); err != nil {
		t.Fatalf("SetStoredToken (new): %v", err)
	}
	restoreStoredToken("cli_test", "ou_alice", prior)
	got := larkauth.GetStoredToken("cli_test", "ou_alice")
	if got == nil {
		t.Fatal("token slot empty after restore")
	}
	if got.AccessToken != "prior-tok" {
		t.Errorf("AccessToken = %q, want prior-tok (restore failed)", got.AccessToken)
	}
}

func TestRestoreStoredToken_PriorAbsent_RemovesNew(t *testing.T) {
	keyring.MockInit()
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_test", UserOpenId: "ou_alice", AccessToken: "new-tok",
	}); err != nil {
		t.Fatalf("SetStoredToken: %v", err)
	}
	restoreStoredToken("cli_test", "ou_alice", nil)
	if got := larkauth.GetStoredToken("cli_test", "ou_alice"); got != nil {
		t.Errorf("token slot non-empty after restore (prior was nil — should have removed): %#v", got)
	}
}

// First login on an empty profile lands the user and stamps CurrentUser.
func TestAuthLoginRun_Step8_FirstUserStampsCurrentUser(t *testing.T) {
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps:       []core.AppConfig{{Name: "default", AppId: "cli_test"}},
	})
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stubLoginHTTP(t, reg, "ou_alice", "Alice")
	f.Invocation = cmdutil.InvocationContext{Profile: "default"}

	if err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send",
	}); err != nil {
		t.Fatalf("authLoginRun: %v", err)
	}
	saved, _ := core.LoadMultiAppConfig()
	if saved.Apps[0].CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser = %q, want ou_alice", saved.Apps[0].CurrentUser)
	}
	if len(saved.Apps[0].Users) != 1 || saved.Apps[0].Users[0].UnionId != "uid_ou_alice" {
		t.Errorf("Users[0] missing union_id capture: %#v", saved.Apps[0].Users)
	}
	if saved.Apps[0].Users[0].FirstAuthAt == nil {
		t.Error("FirstAuthAt must be stamped on first login")
	}
	if saved.Apps[0].Users[0].LastScopes == "" {
		t.Error("LastScopes should reflect granted scopes")
	}
}

// Re-login of the active user updates the row in place; CurrentUser unchanged,
// FirstAuthAt sticky.
func TestAuthLoginRun_Step8_ReLoginActiveUserRefreshes(t *testing.T) {
	firstAuth := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users: []core.AppUser{{
				UserOpenId: "ou_alice", UserName: "old-name",
				FirstAuthAt: &firstAuth,
			}},
		}},
	})
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stubLoginHTTP(t, reg, "ou_alice", "Alice (refreshed)")
	f.Invocation = cmdutil.InvocationContext{Profile: "default"}

	if err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send",
	}); err != nil {
		t.Fatalf("authLoginRun: %v", err)
	}
	saved, _ := core.LoadMultiAppConfig()
	if len(saved.Apps[0].Users) != 1 {
		t.Fatalf("re-login produced duplicate row: %#v", saved.Apps[0].Users)
	}
	u := saved.Apps[0].Users[0]
	if u.UserName != "Alice (refreshed)" {
		t.Errorf("UserName = %q, want refreshed", u.UserName)
	}
	if u.FirstAuthAt == nil || !u.FirstAuthAt.Equal(firstAuth) {
		t.Errorf("FirstAuthAt = %v, want sticky %v", u.FirstAuthAt, firstAuth)
	}
}
