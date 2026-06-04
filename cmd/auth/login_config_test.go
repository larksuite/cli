// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"strings"
	"testing"
	"time"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/zalando/go-keyring"
)

func setupLoginConfigDir(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
}

// testLoginRoot returns an auth.Root rooted at the test's config dir so
// flock + sidecar state stays inside the temp dir.
func testLoginRoot(t *testing.T) larkauth.Root {
	t.Helper()
	return larkauth.NewLocalRoot(core.GetConfigDir())
}

// Upsert must append, not replace: legacy single-user semantics would have
// silently wiped Alice when Bob logged in.
func TestSyncLoginUserToProfile_UpsertNewUserAppendsRow(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "target",
		Apps: []core.AppConfig{
			{
				Name:        "target",
				AppId:       "app-target",
				CurrentUser: "ou_alice",
				Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	var stderr bytes.Buffer
	if err := syncLoginUserToProfile(testLoginRoot(t), "target", "app-target", "ou_bob", "uid_bob", "Bob", "im:message:send", time.Now(), &stderr); err != nil {
		t.Fatalf("syncLoginUserToProfile: %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	users := saved.Apps[0].Users
	if len(users) != 2 {
		t.Fatalf("Users len = %d, want 2 (Alice preserved, Bob appended); got %#v", len(users), users)
	}
	if users[0].UserOpenId != "ou_alice" {
		t.Errorf("Users[0] = %q, want ou_alice (insertion order preserved)", users[0].UserOpenId)
	}
	if users[1].UserOpenId != "ou_bob" || users[1].UserName != "Bob" {
		t.Errorf("Users[1] = %#v, want Bob", users[1])
	}
	if users[1].UnionId != "uid_bob" {
		t.Errorf("Users[1].UnionId = %q, want uid_bob", users[1].UnionId)
	}
	// Re-login of a different user must not switch the active user.
	if saved.Apps[0].CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser = %q, want ou_alice (re-login of a different user must NOT switch active user)", saved.Apps[0].CurrentUser)
	}
}

// Re-login refreshes UserName/LastUsed/LastScopes but FirstAuthAt is sticky.
func TestSyncLoginUserToProfile_UpsertExistingUserUpdatesInPlace(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	firstAuth := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	multi := &core.MultiAppConfig{
		CurrentApp: "target",
		Apps: []core.AppConfig{
			{
				Name:        "target",
				AppId:       "app-target",
				CurrentUser: "ou_alice",
				Users: []core.AppUser{{
					UserOpenId:  "ou_alice",
					UserName:    "old-name",
					FirstAuthAt: &firstAuth,
				}},
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	var stderr bytes.Buffer
	if err := syncLoginUserToProfile(testLoginRoot(t), "target", "app-target", "ou_alice", "uid_alice", "Alice", "docs:read im:message:send", now, &stderr); err != nil {
		t.Fatalf("syncLoginUserToProfile: %v", err)
	}

	saved, _ := core.LoadMultiAppConfig()
	users := saved.Apps[0].Users
	if len(users) != 1 {
		t.Fatalf("Users len = %d, want 1 (re-login should not append duplicate)", len(users))
	}
	u := users[0]
	if u.UserName != "Alice" {
		t.Errorf("UserName = %q, want Alice (refreshed)", u.UserName)
	}
	if u.UnionId != "uid_alice" {
		t.Errorf("UnionId = %q, want uid_alice", u.UnionId)
	}
	if u.FirstAuthAt == nil || !u.FirstAuthAt.Equal(firstAuth) {
		t.Errorf("FirstAuthAt = %v, want %v (must be sticky)", u.FirstAuthAt, firstAuth)
	}
	if u.LastUsed == nil || !u.LastUsed.Equal(now) {
		t.Errorf("LastUsed = %v, want %v", u.LastUsed, now)
	}
	if u.LastScopes != "docs:read,im:message:send" {
		t.Errorf("LastScopes = %q, want sorted+joined form", u.LastScopes)
	}
}

// First login on an empty profile must stamp CurrentUser; `auth users use`
// flows expect it populated.
func TestSyncLoginUserToProfile_FirstUserStampsCurrentUser(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{Name: "target", AppId: "app-target"}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	var stderr bytes.Buffer
	if err := syncLoginUserToProfile(testLoginRoot(t), "target", "app-target", "ou_alice", "", "Alice", "", time.Now(), &stderr); err != nil {
		t.Fatalf("syncLoginUserToProfile: %v", err)
	}
	saved, _ := core.LoadMultiAppConfig()
	if saved.Apps[0].CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser = %q, want ou_alice (first user stamp)", saved.Apps[0].CurrentUser)
	}
}

// Regression guard: re-login of the active user must not clear-and-rewrite
// CurrentUser.
func TestSyncLoginUserToProfile_RefreshDoesNotChangeCurrentUser(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name:        "target",
			AppId:       "app-target",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	var stderr bytes.Buffer
	if err := syncLoginUserToProfile(testLoginRoot(t), "target", "app-target", "ou_alice", "", "Alice", "", time.Now(), &stderr); err != nil {
		t.Fatalf("syncLoginUserToProfile: %v", err)
	}
	saved, _ := core.LoadMultiAppConfig()
	if saved.Apps[0].CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser = %q, want ou_alice (refresh of active user)", saved.Apps[0].CurrentUser)
	}
}

// Touching one profile must not stomp another profile's users.
func TestSyncLoginUserToProfile_PreservesOtherProfiles(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "target",
		Apps: []core.AppConfig{
			{
				Name:  "target",
				AppId: "app-target",
				Users: []core.AppUser{{UserOpenId: "ou_old", UserName: "old"}},
			},
			{
				Name:  "other",
				AppId: "app-other",
				Users: []core.AppUser{{UserOpenId: "ou_other", UserName: "other"}},
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	var stderr bytes.Buffer
	if err := syncLoginUserToProfile(testLoginRoot(t), "target", "app-target", "ou_new", "", "new-user", "", time.Now(), &stderr); err != nil {
		t.Fatalf("syncLoginUserToProfile() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if got := saved.Apps[0].Users; len(got) != 2 {
		t.Fatalf("target users = %#v, want 2 entries (upsert preserves prior)", got)
	}
	if got := saved.Apps[1].Users; len(got) != 1 || got[0].UserOpenId != "ou_other" {
		t.Fatalf("other users = %#v, want unchanged", got)
	}
}

func TestSyncLoginUserToProfile_ProfileNotFoundReturnsError(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name:  "default",
			AppId: "app-default",
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	var stderr bytes.Buffer
	err := syncLoginUserToProfile(testLoginRoot(t), "missing", "app-default", "ou_new", "", "new-user", "", time.Now(), &stderr)
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
	if !strings.Contains(err.Error(), `profile "missing" not found`) {
		t.Fatalf("error = %v, want missing profile", err)
	}
}
