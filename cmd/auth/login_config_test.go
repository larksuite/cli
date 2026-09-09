// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"strings"
	"testing"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/zalando/go-keyring"
)

func setupLoginConfigDir(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
}

func TestSyncLoginUserToProfile_AppendsAndSelectsUserInTargetProfile(t *testing.T) {
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
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{AppId: "app-target", UserOpenId: "ou_old"}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	if err := syncLoginUserToProfile("target", "ou_new", "new-user"); err != nil {
		t.Fatalf("syncLoginUserToProfile() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if got := saved.Apps[0].Users; len(got) != 2 || got[0].UserOpenId != "ou_old" || got[1].UserOpenId != "ou_new" || got[1].UserName != "new-user" {
		t.Fatalf("target users = %#v, want existing user followed by new login", got)
	}
	if got := saved.Apps[0].CurrentUser; got != "ou_new" {
		t.Fatalf("target currentUser = %q, want ou_new", got)
	}
	if got := saved.Apps[1].Users; len(got) != 1 || got[0].UserOpenId != "ou_other" {
		t.Fatalf("other users = %#v, want unchanged", got)
	}
	if larkauth.GetStoredToken("app-target", "ou_old") == nil {
		t.Fatal("existing user's stored token was removed")
	}
}

func TestSyncLoginUserToProfile_ProfileNotFoundReturnsError(t *testing.T) {
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

	err := syncLoginUserToProfile("missing", "ou_new", "new-user")
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
	if !strings.Contains(err.Error(), `profile "missing" not found`) {
		t.Fatalf("error = %v, want missing profile", err)
	}
}

func TestSyncLoginUserToProfile_UpdatesExistingUserWithoutDuplication(t *testing.T) {
	setupLoginConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "target",
		Apps: []core.AppConfig{{
			Name:        "target",
			AppId:       "app-target",
			CurrentUser: "ou_other",
			Users: []core.AppUser{
				{UserOpenId: "ou_existing", UserName: "old-name"},
				{UserOpenId: "ou_other", UserName: "other"},
			},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	if err := syncLoginUserToProfile("target", "ou_existing", "new-name"); err != nil {
		t.Fatalf("syncLoginUserToProfile() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	got := saved.Apps[0]
	if len(got.Users) != 2 || got.Users[0].UserName != "new-name" {
		t.Fatalf("users = %#v, want updated user without duplicate", got.Users)
	}
	if got.CurrentUser != "ou_existing" {
		t.Fatalf("currentUser = %q, want ou_existing", got.CurrentUser)
	}
}
