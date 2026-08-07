// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"strings"
	"testing"

	configpkg "github.com/larksuite/cli/internal/config"
)

func setupLoginConfigDir(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
}

func TestSyncLoginUserToProfile_UpdatesOnlyTargetProfile(t *testing.T) {
	setupLoginConfigDir(t)
	multi := &configpkg.MultiAppConfig{
		CurrentApp: "target",
		Apps: []configpkg.AppConfig{
			{
				Name:  "target",
				AppId: "app-target",
				Users: []configpkg.AppUser{{UserOpenId: "ou_old", UserName: "old"}},
			},
			{
				Name:  "other",
				AppId: "app-other",
				Users: []configpkg.AppUser{{UserOpenId: "ou_other", UserName: "other"}},
			},
		},
	}
	if err := configpkg.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	if err := syncLoginUserToProfile("target", "app-target", "ou_new", "new-user"); err != nil {
		t.Fatalf("syncLoginUserToProfile() error = %v", err)
	}

	saved, err := configpkg.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if got := saved.Apps[0].Users; len(got) != 1 || got[0].UserOpenId != "ou_new" || got[0].UserName != "new-user" {
		t.Fatalf("target users = %#v, want replaced login user", got)
	}
	if got := saved.Apps[1].Users; len(got) != 1 || got[0].UserOpenId != "ou_other" {
		t.Fatalf("other users = %#v, want unchanged", got)
	}
}

func TestSyncLoginUserToProfile_ProfileNotFoundReturnsError(t *testing.T) {
	setupLoginConfigDir(t)
	multi := &configpkg.MultiAppConfig{
		Apps: []configpkg.AppConfig{{
			Name:  "default",
			AppId: "app-default",
		}},
	}
	if err := configpkg.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	err := syncLoginUserToProfile("missing", "app-default", "ou_new", "new-user")
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
	if !strings.Contains(err.Error(), `profile "missing" not found`) {
		t.Fatalf("error = %v, want missing profile", err)
	}
}
