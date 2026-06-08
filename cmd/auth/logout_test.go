// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// stubLogoutConfig writes a `target` profile with the given users and pre-stashes
// keychain token + sidecar profile + index row for each, so logout has state to wipe.
func stubLogoutConfig(t *testing.T, currentUser string, users []core.AppUser) (*cmdutil.Factory, string, string) {
	t.Helper()
	keyring.MockInit()
	cfgDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", cfgDir)
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

	root := larkauth.NewLocalRoot(cfgDir)
	for _, u := range users {
		if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
			AppId: "app-target", UserOpenId: u.UserOpenId, AccessToken: "tok-" + u.UserOpenId,
		}); err != nil {
			t.Fatalf("SetStoredToken(%s): %v", u.UserOpenId, err)
		}
		ctx := larkauth.ForUser("app-target", u.UserOpenId)
		now := time.Now().UTC()
		if err := larkauth.SaveUserProfileFor(root, ctx, larkauth.UserProfile{
			UserOpenId: u.UserOpenId, UserName: u.UserName, CachedAt: now, FirstAuthAt: now,
		}); err != nil {
			t.Fatalf("SaveUserProfileFor(%s): %v", u.UserOpenId, err)
		}
		if err := larkauth.RecordUserActivity(root, ctx, nil); err != nil {
			t.Fatalf("RecordUserActivity(%s): %v", u.UserOpenId, err)
		}
	}

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "target", AppID: "app-target", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	f.Invocation = cmdutil.InvocationContext{Profile: "target"}
	return f, cfgDir, "app-target"
}

// userIndexHas reports whether users.json contains a row for openID. Uses the
// public Root API so the test is robust to on-disk format changes.
func userIndexHas(t *testing.T, cfgDir, appID, openID string) bool {
	t.Helper()
	root := larkauth.NewLocalRoot(cfgDir)
	all, err := larkauth.UserIndexEntries(root)
	if err != nil {
		t.Fatalf("UserIndexEntries: %v", err)
	}
	for _, e := range all {
		if e.AppId == appID && e.UserOpenId == openID {
			return true
		}
	}
	return false
}

// Short-circuits before flock.
func TestAuthLogoutRun_NoConfig(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})

	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("authLogoutRun: %v", err)
	}
	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()
	if !strings.Contains(stderr, "No configuration found") {
		t.Errorf("stderr = %q, want 'No configuration found.'", stderr)
	}
}

// TestAuthLogoutRun_NotLoggedIn: profile exists, no users.
func TestAuthLogoutRun_NotLoggedIn(t *testing.T) {
	f, _, _ := stubLogoutConfig(t, "", nil)
	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("authLogoutRun: %v", err)
	}
	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()
	if !strings.Contains(stderr, "Not logged in") {
		t.Errorf("stderr = %q, want 'Not logged in.'", stderr)
	}
}

// Headline behavior: keychain, config, sidecar JSON, and index row are all cleared.
func TestAuthLogoutRun_WipesKeychainConfigSidecarAndIndex(t *testing.T) {
	f, cfgDir, appID := stubLogoutConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
		{UserOpenId: "ou_bob", UserName: "Bob"},
	})

	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("authLogoutRun: %v", err)
	}

	// Config: Users empty, CurrentUser cleared.
	saved, _ := core.LoadMultiAppConfig()
	if len(saved.Apps[0].Users) != 0 {
		t.Errorf("Users = %#v, want empty", saved.Apps[0].Users)
	}
	if saved.Apps[0].CurrentUser != "" {
		t.Errorf("CurrentUser = %q, want empty", saved.Apps[0].CurrentUser)
	}

	// Keychain: both users gone.
	for _, openID := range []string{"ou_alice", "ou_bob"} {
		if got := larkauth.GetStoredToken(appID, openID); got != nil {
			t.Errorf("token for %s still present: %#v", openID, got)
		}
	}

	// Sidecar profile JSONs: gone for both.
	for _, openID := range []string{"ou_alice", "ou_bob"} {
		path := filepath.Join(cfgDir, "users", appID, openID, "user_profile.json")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("sidecar %s still present (err=%v)", path, err)
		}
	}

	// User index rows: gone for both.
	for _, openID := range []string{"ou_alice", "ou_bob"} {
		if userIndexHas(t, cfgDir, appID, openID) {
			t.Errorf("index row %s still present", openID)
		}
	}

	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()
	if !strings.Contains(stderr, "Logged out 2 users") {
		t.Errorf("stderr = %q, want 'Logged out 2 users'", stderr)
	}
}

// Success line names the user when only one was wiped.
func TestAuthLogoutRun_SingleUserPhrasing(t *testing.T) {
	f, _, _ := stubLogoutConfig(t, "ou_solo", []core.AppUser{
		{UserOpenId: "ou_solo", UserName: "Solo"},
	})
	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("authLogoutRun: %v", err)
	}
	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()
	if !strings.Contains(stderr, "Solo") || !strings.Contains(stderr, "ou_solo") {
		t.Errorf("stderr = %q, want phrasing naming Solo (ou_solo)", stderr)
	}
}

// Wiping `target` must not touch a sibling profile's users/keychain/index.
func TestAuthLogoutRun_PreservesOtherProfiles(t *testing.T) {
	keyring.MockInit()
	cfgDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", cfgDir)
	t.Setenv("HOME", t.TempDir())

	multi := &core.MultiAppConfig{
		CurrentApp: "target",
		Apps: []core.AppConfig{
			{
				Name: "target", AppId: "app-target", Brand: core.BrandFeishu,
				CurrentUser: "ou_alice",
				Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
			},
			{
				Name: "other", AppId: "app-other", Brand: core.BrandFeishu,
				CurrentUser: "ou_carol",
				Users:       []core.AppUser{{UserOpenId: "ou_carol", UserName: "Carol"}},
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}
	root := larkauth.NewLocalRoot(cfgDir)
	for _, u := range []struct{ app, oid, name string }{
		{"app-target", "ou_alice", "Alice"},
		{"app-other", "ou_carol", "Carol"},
	} {
		_ = larkauth.SetStoredToken(&larkauth.StoredUAToken{AppId: u.app, UserOpenId: u.oid, AccessToken: "tok"})
		ctx := larkauth.ForUser(u.app, u.oid)
		now := time.Now().UTC()
		_ = larkauth.SaveUserProfileFor(root, ctx, larkauth.UserProfile{UserOpenId: u.oid, UserName: u.name, CachedAt: now, FirstAuthAt: now})
		_ = larkauth.RecordUserActivity(root, ctx, nil)
	}

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "target", AppID: "app-target", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	f.Invocation = cmdutil.InvocationContext{Profile: "target"}

	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("authLogoutRun: %v", err)
	}

	saved, _ := core.LoadMultiAppConfig()
	// `target` wiped.
	if len(saved.Apps[0].Users) != 0 {
		t.Errorf("target.Users = %#v, want empty", saved.Apps[0].Users)
	}
	// `other` untouched.
	if len(saved.Apps[1].Users) != 1 || saved.Apps[1].Users[0].UserOpenId != "ou_carol" {
		t.Errorf("other.Users = %#v, want Carol preserved", saved.Apps[1].Users)
	}
	if saved.Apps[1].CurrentUser != "ou_carol" {
		t.Errorf("other.CurrentUser = %q, want ou_carol preserved", saved.Apps[1].CurrentUser)
	}
	// Carol's keychain + sidecar + index row preserved.
	if got := larkauth.GetStoredToken("app-other", "ou_carol"); got == nil {
		t.Error("Carol's token wiped")
	}
	if !userIndexHas(t, cfgDir, "app-other", "ou_carol") {
		t.Error("Carol's index row wiped")
	}
}

// Running logout twice is a no-op the second time and does not error.
func TestAuthLogoutRun_IsIdempotent(t *testing.T) {
	f, _, _ := stubLogoutConfig(t, "ou_alice", []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}})
	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("first logout: %v", err)
	}
	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("second logout: %v", err)
	}
	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()
	if !strings.Contains(stderr, "Not logged in") {
		t.Errorf("second logout stderr = %q, want 'Not logged in.'", stderr)
	}
}
