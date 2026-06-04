// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"testing"

	"github.com/zalando/go-keyring"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// Regression: pre-fix, profileRemoveRun cleaned up keychain UATs but
// left disk-backed sidecar profiles and the install-wide
// user_index.json row in place. Result: a removed profile's users
// would resurface in `auth users list` and mis-attribute the slot on
// re-login under the same open_id.
func TestProfileRemoveRun_SweepsAllUserArtifacts(t *testing.T) {
	keyring.MockInit()
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	multi := &core.MultiAppConfig{
		CurrentApp: "keep",
		Apps: []core.AppConfig{
			{
				Name:  "keep",
				AppId: "cli_keep",
				AppSecret: core.SecretInput{
					Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_keep"},
				},
				Brand: core.BrandFeishu,
				Users: []core.AppUser{{UserOpenId: "ou_keeper", UserName: "Keeper"}},
			},
			{
				Name:  "victim",
				AppId: "cli_victim",
				AppSecret: core.SecretInput{
					Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_victim"},
				},
				Brand: core.BrandFeishu,
				Users: []core.AppUser{
					{UserOpenId: "ou_alice", UserName: "Alice"},
					{UserOpenId: "ou_bob", UserName: "Bob"},
				},
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	root := larkauth.NewLocalRoot(configDir)

	// Seed all three artifact legs for both victim users.
	for _, u := range []string{"ou_alice", "ou_bob"} {
		ctx := larkauth.ForUser("cli_victim", u)
		if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
			AppId: "cli_victim", UserOpenId: u, AccessToken: "tok_" + u,
		}); err != nil {
			t.Fatalf("seed UAT for %s: %v", u, err)
		}
		if err := larkauth.SaveUserProfileFor(root, ctx, larkauth.UserProfile{
			UserOpenId: u, UserName: u,
		}); err != nil {
			t.Fatalf("seed sidecar for %s: %v", u, err)
		}
		if err := larkauth.RecordUserActivity(root, ctx, nil); err != nil {
			t.Fatalf("seed index for %s: %v", u, err)
		}
	}

	// Seed the keeper too, to prove they survive the operation.
	keepCtx := larkauth.ForUser("cli_keep", "ou_keeper")
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_keep", UserOpenId: "ou_keeper", AccessToken: "tok_keep",
	}); err != nil {
		t.Fatalf("seed keeper UAT: %v", err)
	}
	if err := larkauth.SaveUserProfileFor(root, keepCtx, larkauth.UserProfile{
		UserOpenId: "ou_keeper", UserName: "Keeper",
	}); err != nil {
		t.Fatalf("seed keeper sidecar: %v", err)
	}
	if err := larkauth.RecordUserActivity(root, keepCtx, nil); err != nil {
		t.Fatalf("seed keeper index: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := profileRemoveRun(f, "victim"); err != nil {
		t.Fatalf("profileRemoveRun: %v", err)
	}

	// Victim users: every leg must be empty.
	for _, u := range []string{"ou_alice", "ou_bob"} {
		ctx := larkauth.ForUser("cli_victim", u)
		if got := larkauth.GetStoredToken("cli_victim", u); got != nil {
			t.Errorf("victim UAT (cli_victim, %s) not removed: %+v", u, got)
		}
		if got, _ := larkauth.LoadUserProfileFor(root, ctx); got != nil {
			t.Errorf("victim sidecar (cli_victim, %s) not removed: %+v", u, got)
		}
	}
	idx, _ := larkauth.LoadUserIndex(root)
	for _, e := range idx.Users {
		if e.AppId == "cli_victim" {
			t.Errorf("victim index row not removed: %+v", e)
		}
	}

	// Keeper: every leg must survive.
	if got := larkauth.GetStoredToken("cli_keep", "ou_keeper"); got == nil {
		t.Errorf("keeper UAT was wiped")
	}
	if got, _ := larkauth.LoadUserProfileFor(root, keepCtx); got == nil {
		t.Errorf("keeper sidecar was wiped")
	}
	keeperFound := false
	for _, e := range idx.Users {
		if e.AppId == "cli_keep" && e.UserOpenId == "ou_keeper" {
			keeperFound = true
		}
	}
	if !keeperFound {
		t.Errorf("keeper index row was wiped; idx=%+v", idx.Users)
	}
}
