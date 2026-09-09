// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/zalando/go-keyring"
)

func setupUsersConfig(t *testing.T) *core.MultiAppConfig {
	t.Helper()
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			AppSecret:   core.PlainSecret("secret"),
			Brand:       core.BrandFeishu,
			CurrentUser: "ou_second",
			Users: []core.AppUser{
				{UserOpenId: "ou_first", UserName: "first"},
				{UserOpenId: "ou_second", UserName: "second"},
			},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
	return multi
}

func TestAuthUsersListRunMarksActiveUser(t *testing.T) {
	setupUsersConfig(t)
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:      "cli_test",
		UserOpenId: "ou_second",
		ExpiresAt:  1<<62 - 1,
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	if err := authUsersListRun(&UsersListOptions{Factory: f}); err != nil {
		t.Fatalf("authUsersListRun() error = %v", err)
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("stdout must be JSON: %v\n%s", err, stdout.String())
	}
	if len(rows) != 2 || rows[0]["active"] != false || rows[1]["active"] != true {
		t.Fatalf("rows = %#v, want only second user active", rows)
	}
	if rows[1]["tokenStatus"] != "valid" {
		t.Fatalf("second tokenStatus = %v, want valid", rows[1]["tokenStatus"])
	}
}

func TestAuthUsersUseRunSwitchesCurrentUser(t *testing.T) {
	setupUsersConfig(t)
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)

	if err := authUsersUseRun(&UsersUseOptions{Factory: f, Target: "first"}); err != nil {
		t.Fatalf("authUsersUseRun() error = %v", err)
	}
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.Apps[0].CurrentUser != "ou_first" {
		t.Fatalf("CurrentUser = %q, want ou_first", saved.Apps[0].CurrentUser)
	}
	if !strings.Contains(stderr.String(), "Active user: first (ou_first)") {
		t.Fatalf("stderr = %q, want active user confirmation", stderr.String())
	}
}

func TestAuthUsersUseRunRejectsAmbiguousName(t *testing.T) {
	multi := setupUsersConfig(t)
	multi.Apps[0].Users[0].UserName = "same"
	multi.Apps[0].Users[1].UserName = "same"
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := authUsersUseRun(&UsersUseOptions{Factory: f, Target: "same"})
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || !strings.Contains(validationErr.Message, "ambiguous") {
		t.Fatalf("error = %T %v, want ambiguous ValidationError", err, err)
	}
}

func TestAuthUsersLogoutRunRemovesOnlyTargetAndSelectsFallback(t *testing.T) {
	setupUsersConfig(t)
	for _, openID := range []string{"ou_first", "ou_second"} {
		if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
			AppId:      "cli_test",
			UserOpenId: openID,
		}); err != nil {
			t.Fatalf("SetStoredToken(%s) error = %v", openID, err)
		}
	}

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authUsersLogoutRun(&UsersLogoutOptions{Factory: f, Target: "ou_second"}); err != nil {
		t.Fatalf("authUsersLogoutRun() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	app := saved.Apps[0]
	if len(app.Users) != 1 || app.Users[0].UserOpenId != "ou_first" || app.CurrentUser != "ou_first" {
		t.Fatalf("app = %#v, want only ou_first active", app)
	}
	if larkauth.GetStoredToken("cli_test", "ou_second") != nil {
		t.Fatal("target token still exists")
	}
	if larkauth.GetStoredToken("cli_test", "ou_first") == nil {
		t.Fatal("non-target token was removed")
	}
	if !strings.Contains(stderr.String(), "Active user: first (ou_first)") {
		t.Fatalf("stderr = %q, want fallback identity", stderr.String())
	}
}

func TestAuthUsersCommandsParseTargets(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	var useTarget, logoutTarget string
	use := NewCmdAuthUsersUse(f, func(opts *UsersUseOptions) error {
		useTarget = opts.Target
		return nil
	})
	use.SetArgs([]string{"ou_use"})
	if err := use.Execute(); err != nil {
		t.Fatalf("users use execute error = %v", err)
	}
	logout := NewCmdAuthUsersLogout(f, func(opts *UsersLogoutOptions) error {
		logoutTarget = opts.Target
		return nil
	})
	logout.SetArgs([]string{"ou_logout"})
	if err := logout.Execute(); err != nil {
		t.Fatalf("users logout execute error = %v", err)
	}
	if useTarget != "ou_use" || logoutTarget != "ou_logout" {
		t.Fatalf("targets = use:%q logout:%q", useTarget, logoutTarget)
	}
}
