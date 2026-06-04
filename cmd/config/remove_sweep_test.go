// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/zalando/go-keyring"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// Regression: pre-fix, configRemoveRun cleared keychain UATs but left
// the disk-backed sidecar profile (per-user user_profile.json) and the
// install-wide user_index.json row in place. Result:
//
//   - `auth users list` keeps showing the removed user (loads from
//     user_index.json).
//   - A subsequent re-login by a different human under the same
//     open_id mis-attributes the slot (UserName, FirstAuthAt, etc.
//     are pulled from the stale sidecar).
//
// This test seeds all three legs for a removed user, runs the config
// remove command, and proves all three legs are gone afterwards.
func TestConfigRemoveRun_SweepsAllUserArtifacts(t *testing.T) {
	keyring.MockInit()
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_app",
			AppSecret: core.SecretInput{
				Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_app"},
			},
			Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	// Seed every user-artifact leg.
	root := larkauth.NewLocalRoot(configDir)
	ctx := larkauth.ForUser("cli_app", "ou_alice")
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_app", UserOpenId: "ou_alice", AccessToken: "tok",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}
	if err := larkauth.SaveUserProfileFor(root, ctx, larkauth.UserProfile{
		UserOpenId: "ou_alice", UserName: "Alice",
	}); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}
	if err := larkauth.RecordUserActivity(root, ctx, []string{"im:message:send"}); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := configRemoveRun(&ConfigRemoveOptions{Factory: f}); err != nil {
		t.Fatalf("configRemoveRun: %v", err)
	}

	if got := larkauth.GetStoredToken("cli_app", "ou_alice"); got != nil {
		t.Errorf("keychain UAT not removed: %+v", got)
	}
	if got, err := larkauth.LoadUserProfileFor(root, ctx); err != nil {
		t.Fatalf("LoadUserProfileFor: %v", err)
	} else if got != nil {
		t.Errorf("sidecar profile not removed: %+v", got)
	}
	idx, err := larkauth.LoadUserIndex(root)
	if err != nil {
		t.Fatalf("LoadUserIndex: %v", err)
	}
	for _, e := range idx.Users {
		if e.AppId == "cli_app" && e.UserOpenId == "ou_alice" {
			t.Errorf("index row not removed: %+v", e)
		}
	}
}

// TestCleanupOldConfig_SweepsAllUserArtifacts targets cmd/config/init.go's
// cleanupOldConfig — the config init "I'm replacing this whole config
// with a different app" path. Same disease, different remove site.
func TestCleanupOldConfig_SweepsAllUserArtifacts(t *testing.T) {
	keyring.MockInit()
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	root := larkauth.NewLocalRoot(configDir)
	ctx := larkauth.ForUser("cli_old", "ou_alice")
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_old", UserOpenId: "ou_alice", AccessToken: "tok",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}
	if err := larkauth.SaveUserProfileFor(root, ctx, larkauth.UserProfile{
		UserOpenId: "ou_alice", UserName: "Alice",
	}); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}
	if err := larkauth.RecordUserActivity(root, ctx, nil); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_old",
			AppSecret: core.SecretInput{
				Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_old"},
			},
			Users: []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	// Skip target is a different appId, so the old config IS swept.
	cleanupOldConfig(existing, f, "cli_brand_new")

	if got := larkauth.GetStoredToken("cli_old", "ou_alice"); got != nil {
		t.Errorf("keychain UAT not removed: %+v", got)
	}
	if got, _ := larkauth.LoadUserProfileFor(root, ctx); got != nil {
		t.Errorf("sidecar profile not removed: %+v", got)
	}
	idx, err := larkauth.LoadUserIndex(root)
	if err != nil {
		t.Fatalf("LoadUserIndex: %v", err)
	}
	for _, e := range idx.Users {
		if e.AppId == "cli_old" && e.UserOpenId == "ou_alice" {
			t.Errorf("index row not removed: %+v", e)
		}
	}
}

// TestCleanupOldConfig_SkipPreservesAllUserArtifacts — when an app is
// the skip target (operator wants to keep it), NONE of its artifacts
// are touched.
func TestCleanupOldConfig_SkipPreservesAllUserArtifacts(t *testing.T) {
	keyring.MockInit()
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	root := larkauth.NewLocalRoot(configDir)
	ctx := larkauth.ForUser("cli_keep", "ou_alice")
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_keep", UserOpenId: "ou_alice", AccessToken: "tok",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}
	if err := larkauth.SaveUserProfileFor(root, ctx, larkauth.UserProfile{
		UserOpenId: "ou_alice", UserName: "Alice",
	}); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}
	if err := larkauth.RecordUserActivity(root, ctx, nil); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "cli_keep",
			AppSecret: core.SecretInput{
				Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_keep"},
			},
			Users: []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cleanupOldConfig(existing, f, "cli_keep")

	if got := larkauth.GetStoredToken("cli_keep", "ou_alice"); got == nil {
		t.Errorf("keychain UAT was removed for skip-target")
	}
	if got, _ := larkauth.LoadUserProfileFor(root, ctx); got == nil {
		t.Errorf("sidecar profile was removed for skip-target")
	}
	idx, _ := larkauth.LoadUserIndex(root)
	found := false
	for _, e := range idx.Users {
		if e.AppId == "cli_keep" && e.UserOpenId == "ou_alice" {
			found = true
		}
	}
	if !found {
		t.Errorf("index row was removed for skip-target; idx=%+v", idx.Users)
	}
}

// TestCleanupKeychainFromData_SweepsSidecarAndIndex extends the B3
// regression coverage: the sweep now removes sidecar + index in
// addition to the keychain UAT. Pre-fix, only the UAT went away.
func TestCleanupKeychainFromData_SweepsSidecarAndIndex(t *testing.T) {
	keyring.MockInit()
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	root := larkauth.NewLocalRoot(configDir)
	ctx := larkauth.ForUser("cli_old", "ou_alice")
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_old", UserOpenId: "ou_alice", AccessToken: "tok",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}
	if err := larkauth.SaveUserProfileFor(root, ctx, larkauth.UserProfile{
		UserOpenId: "ou_alice", UserName: "Alice",
	}); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}
	if err := larkauth.RecordUserActivity(root, ctx, nil); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	oldConfig := []byte(`{"apps":[{"appId":"cli_old","appSecret":{"source":"keychain","id":"appsecret:cli_old"},"users":[{"userOpenId":"ou_alice","userName":"Alice"}]}]}`)
	newApp := &core.AppConfig{
		AppId:     "cli_new",
		AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_new"}},
		Users:     []core.AppUser{},
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cleanupKeychainFromData(f.Keychain, oldConfig, newApp)

	if got := larkauth.GetStoredToken("cli_old", "ou_alice"); got != nil {
		t.Errorf("UAT not removed: %+v", got)
	}
	if got, _ := larkauth.LoadUserProfileFor(root, ctx); got != nil {
		t.Errorf("sidecar not removed: %+v", got)
	}
	idx, _ := larkauth.LoadUserIndex(root)
	for _, e := range idx.Users {
		if e.AppId == "cli_old" && e.UserOpenId == "ou_alice" {
			t.Errorf("index row not removed: %+v", e)
		}
	}
}
