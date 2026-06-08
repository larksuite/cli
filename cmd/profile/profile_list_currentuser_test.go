// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// Regression: `profile list` previously hard-coded Users[0] when
// rendering the per-profile "user" column. After `auth users use bob`
// switched the profile's CurrentUser to bob, `profile list` still
// showed alice (Users[0]) — the active-user semantic diverged across
// `auth users list` (which honored CurrentUser) and `profile list`.
//
// Fix mirrors resolveActiveUserOpenId: CurrentUser → Users[0]
// fallback. A stale CurrentUser (not in Users[]) falls back to
// Users[0] rather than emitting an unknown name.
func TestProfileListRun_HonorsCurrentUser(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "target",
		Apps: []core.AppConfig{{
			Name:      "target",
			AppId:     "app-target",
			AppSecret: core.PlainSecret("s"),
			Brand:     core.BrandFeishu,
			Users: []core.AppUser{
				{UserOpenId: "ou_alice", UserName: "Alice"},
				{UserOpenId: "ou_bob", UserName: "Bob"},
			},
			CurrentUser: "ou_bob",
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileListRun(f); err != nil {
		t.Fatalf("profileListRun: %v", err)
	}

	var got []profileListItem
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v; output=%s", err, stdout.String())
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].User != "Bob" {
		t.Errorf("User = %q, want Bob (CurrentUser); profile list ignored CurrentUser and pinned Users[0]", got[0].User)
	}
}

// Stale CurrentUser (no longer in Users[]) falls back to Users[0]
// rather than rendering a phantom "" user. Mirrors
// resolveActiveUserOpenId in cmd/auth/users_list.go.
func TestProfileListRun_StaleCurrentUser_FallsBackToUsersZero(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "target",
		Apps: []core.AppConfig{{
			Name:      "target",
			AppId:     "app-target",
			AppSecret: core.PlainSecret("s"),
			Brand:     core.BrandFeishu,
			Users: []core.AppUser{
				{UserOpenId: "ou_alice", UserName: "Alice"},
			},
			CurrentUser: "ou_ghost", // dangling reference
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileListRun(f); err != nil {
		t.Fatalf("profileListRun: %v", err)
	}
	var got []profileListItem
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got[0].User != "Alice" {
		t.Errorf("stale CurrentUser must fall back to Users[0]; got %q, want Alice", got[0].User)
	}
}
