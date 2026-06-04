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

// Regression: pre-fix, cleanupKeychainFromData on `config bind` cleaned
// up only the AppSecret keychain entry. Per-user UATs were left
// orphaned in the keychain forever, because every binder.Build today
// returns Users: [], so commitBinding wholesale-replaces Apps[] (and
// the new app's Users[] is always empty). Symptom in the wild: users
// rebinding from app A to app B keep accumulating dead tokens under
// the lark-cli service in their OS keychain — observed dozens of
// stale entries from repeated rebinds.
//
// The fix extends cleanupKeychainFromData to sweep every (appId,
// userOpenId) UAT in the previous config, with a per-pair skip set so
// a future binder that propagates Users[] to the new config does not
// destroy still-referenced tokens.

// TestCleanupKeychainFromData_RemovesStaleUAT_OnRebindToNewApp covers
// the dominant real case: rebinding from app A to a different app B,
// where A had logged-in users. Their UATs must be swept.
func TestCleanupKeychainFromData_RemovesStaleUAT_OnRebindToNewApp(t *testing.T) {
	keyring.MockInit()

	// Seed prior UATs for two users under the OLD app.
	for _, u := range []string{"ou_alice", "ou_bob"} {
		if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
			AppId: "cli_old", UserOpenId: u, AccessToken: "tok_" + u,
		}); err != nil {
			t.Fatalf("seed UAT for %s: %v", u, err)
		}
	}

	oldConfig := []byte(`{"apps":[{"appId":"cli_old","appSecret":{"source":"keychain","id":"appsecret:cli_old"},"users":[{"userOpenId":"ou_alice","userName":"Alice"},{"userOpenId":"ou_bob","userName":"Bob"}]}]}`)
	// New app: different appId entirely, fresh Users[] (matches binder.Build).
	newApp := &core.AppConfig{
		AppId:     "cli_new",
		AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_new"}},
		Users:     []core.AppUser{},
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cleanupKeychainFromData(f.Keychain, oldConfig, newApp)

	for _, u := range []string{"ou_alice", "ou_bob"} {
		if got := larkauth.GetStoredToken("cli_old", u); got != nil {
			t.Errorf("UAT for (cli_old, %s) was not removed: %+v", u, got)
		}
	}
}

// TestCleanupKeychainFromData_RemovesStaleUAT_OnRebindToSameAppEmptyUsers
// covers the today-binders case: rebinding to the SAME appId, but
// binder.Build returned Users: [] so the new config has no users. All
// prior users' UATs under that appId must be swept (the new config no
// longer references any of them).
//
// This is what happens whenever a workspace rebinds without any
// upstream change to the source's user list — and it is the
// reproducer for the orphan accumulation observed in production.
func TestCleanupKeychainFromData_RemovesStaleUAT_OnRebindToSameAppEmptyUsers(t *testing.T) {
	keyring.MockInit()

	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_shared", UserOpenId: "ou_alice", AccessToken: "tok_a",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}

	oldConfig := []byte(`{"apps":[{"appId":"cli_shared","appSecret":{"source":"keychain","id":"appsecret:cli_shared"},"users":[{"userOpenId":"ou_alice","userName":"Alice"}]}]}`)
	newApp := &core.AppConfig{
		AppId:     "cli_shared",
		AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_shared"}},
		Users:     []core.AppUser{}, // matches binder.Build today
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cleanupKeychainFromData(f.Keychain, oldConfig, newApp)

	if got := larkauth.GetStoredToken("cli_shared", "ou_alice"); got != nil {
		t.Errorf("stale UAT for (cli_shared, ou_alice) was not removed: %+v", got)
	}
}

// TestCleanupKeychainFromData_PreservesUATWhenNewConfigStillReferencesUser
// is the forward-compat lock: if a future binder propagates Users[] to
// the new config (or a hand-crafted keep argument carries the same
// (appId, userOpenId) pair), the corresponding UAT must NOT be
// destroyed. The skip set is per-(appId, userOpenId), not per-app.
func TestCleanupKeychainFromData_PreservesUATWhenNewConfigStillReferencesUser(t *testing.T) {
	keyring.MockInit()

	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_shared", UserOpenId: "ou_alice", AccessToken: "tok_keep",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}
	// A second user that will be removed (in old config but NOT in keep.Users).
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_shared", UserOpenId: "ou_bob", AccessToken: "tok_drop",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}

	oldConfig := []byte(`{"apps":[{"appId":"cli_shared","appSecret":{"source":"keychain","id":"appsecret:cli_shared"},"users":[{"userOpenId":"ou_alice","userName":"Alice"},{"userOpenId":"ou_bob","userName":"Bob"}]}]}`)
	newApp := &core.AppConfig{
		AppId:     "cli_shared",
		AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_shared"}},
		Users:     []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cleanupKeychainFromData(f.Keychain, oldConfig, newApp)

	// ou_alice survives — the new config still references this user.
	if got := larkauth.GetStoredToken("cli_shared", "ou_alice"); got == nil {
		t.Errorf("UAT for (cli_shared, ou_alice) was destroyed despite being in keep.Users")
	}
	// ou_bob must go — it's only in the old config.
	if got := larkauth.GetStoredToken("cli_shared", "ou_bob"); got != nil {
		t.Errorf("UAT for (cli_shared, ou_bob) was not removed: %+v", got)
	}
}

// TestCleanupKeychainFromData_DoesNotTouchUnrelatedUAT confirms the
// sweep is scoped: a UAT keyed under a DIFFERENT appId entirely (e.g.
// for an unrelated workspace/profile/app the user has logged into via
// some other CLI surface) is not collateral damage of this cleanup.
func TestCleanupKeychainFromData_DoesNotTouchUnrelatedUAT(t *testing.T) {
	keyring.MockInit()

	// UAT for an app that is NOT in the old config under cleanup.
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_unrelated", UserOpenId: "ou_carol", AccessToken: "tok_unrelated",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}

	oldConfig := []byte(`{"apps":[{"appId":"cli_old","appSecret":{"source":"keychain","id":"appsecret:cli_old"},"users":[{"userOpenId":"ou_alice","userName":"Alice"}]}]}`)
	newApp := &core.AppConfig{
		AppId:     "cli_new",
		AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:cli_new"}},
		Users:     []core.AppUser{},
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cleanupKeychainFromData(f.Keychain, oldConfig, newApp)

	if got := larkauth.GetStoredToken("cli_unrelated", "ou_carol"); got == nil {
		t.Errorf("unrelated UAT (cli_unrelated, ou_carol) was destroyed; sweep scope is too broad")
	}
}

// TestCleanupKeychainFromData_NilKeep is the all-old-removed case:
// when there is no new app to keep (caller passes keep=nil — e.g. a
// future "config unbind"), every UAT in the old config is swept.
func TestCleanupKeychainFromData_NilKeep(t *testing.T) {
	keyring.MockInit()

	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId: "cli_old", UserOpenId: "ou_alice", AccessToken: "tok",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}

	oldConfig := []byte(`{"apps":[{"appId":"cli_old","appSecret":{"source":"keychain","id":"appsecret:cli_old"},"users":[{"userOpenId":"ou_alice","userName":"Alice"}]}]}`)
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	cleanupKeychainFromData(f.Keychain, oldConfig, nil)

	if got := larkauth.GetStoredToken("cli_old", "ou_alice"); got != nil {
		t.Errorf("UAT not removed when keep=nil: %+v", got)
	}
}
