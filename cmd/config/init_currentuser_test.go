// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

// Regression: saveAsProfile previously wiped Users[] when AppId
// changed under an existing profile but left CurrentUser pointing
// at the old user_open_id. The dangling reference is invisible to
// `auth users list` (which reads Users[]) but very visible to
// ResolveConfigFromMulti's three-rung fallback — it walks past the
// stale CurrentUser and lands on Users[0]==nil, falling through to
// the "no users" branch silently. After a subsequent `auth login`,
// syncLoginUserToProfile saw a non-empty CurrentUser and skipped
// the empty-CurrentUser branch (which is the only branch that
// stamps the freshly-logged-in user as active for an empty
// profile), so the new user was added to Users[] but never
// activated.
//
// Fix: clear CurrentUser when we wipe Users[] for an AppId pivot.
func TestSaveAsProfile_AppIdChange_ClearsCurrentUser(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name:      "prod",
			AppId:     "cli_old",
			AppSecret: core.PlainSecret("s-old"),
			Brand:     core.BrandFeishu,
			Users: []core.AppUser{
				{UserOpenId: "ou_alice", UserName: "Alice"},
			},
			CurrentUser: "ou_alice",
		}},
	}

	if err := saveAsProfile(existing,
		keychain.KeychainAccess(&noopConfigKeychain{}),
		"prod", "cli_new",
		core.PlainSecret("s-new"),
		core.BrandFeishu, "en",
	); err != nil {
		t.Fatalf("saveAsProfile: %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	app := saved.FindApp("prod")
	if app == nil {
		t.Fatal("prod profile vanished")
	}
	if app.AppId != "cli_new" {
		t.Errorf("AppId = %q, want cli_new", app.AppId)
	}
	if len(app.Users) != 0 {
		t.Errorf("Users = %v, want [] (sweep on AppId change)", app.Users)
	}
	if app.CurrentUser != "" {
		t.Errorf("CurrentUser = %q, want \"\" (must be cleared with the Users sweep — dangling open_id otherwise)", app.CurrentUser)
	}
}

// Counter-test: same AppId update path (Brand-only / Lang-only edit)
// must NOT touch CurrentUser. The clear is targeted at the AppId
// pivot, not a blanket reset on every profile edit.
func TestSaveAsProfile_SameAppId_PreservesCurrentUser(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name:      "prod",
			AppId:     "cli_x",
			AppSecret: core.PlainSecret("s-old"),
			Brand:     core.BrandFeishu,
			Users: []core.AppUser{
				{UserOpenId: "ou_alice", UserName: "Alice"},
			},
			CurrentUser: "ou_alice",
		}},
	}

	if err := saveAsProfile(existing,
		keychain.KeychainAccess(&noopConfigKeychain{}),
		"prod", "cli_x", // same AppId
		core.PlainSecret("s-new"),
		core.BrandLark, "en",
	); err != nil {
		t.Fatalf("saveAsProfile: %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	app := saved.FindApp("prod")
	if app == nil {
		t.Fatal("prod vanished")
	}
	if len(app.Users) != 1 || app.Users[0].UserOpenId != "ou_alice" {
		t.Errorf("Users wiped on same-AppId update: %v", app.Users)
	}
	if app.CurrentUser != "ou_alice" {
		t.Errorf("CurrentUser cleared on same-AppId update: got %q, want ou_alice", app.CurrentUser)
	}
}
