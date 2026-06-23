// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/zalando/go-keyring"
)

func writeLogoutConfig(t *testing.T, users []core.AppUser) {
	t.Helper()
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: "test-app",
		Apps: []core.AppConfig{
			{
				AppId:     "test-app",
				AppSecret: core.PlainSecret("test-secret"),
				Brand:     core.BrandFeishu,
				Users:     users,
			},
		},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
}

func TestAuthLogoutRun_JSONMode_NotConfigured_WritesStdoutOnly(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authLogoutRun(&LogoutOptions{Factory: f, JSON: true}); err != nil {
		t.Fatalf("authLogoutRun() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != true {
		t.Errorf("stdout.ok = %v, want true", payload["ok"])
	}
	if payload["loggedOut"] != false {
		t.Errorf("stdout.loggedOut = %v, want false", payload["loggedOut"])
	}
	if payload["reason"] != "not_configured" {
		t.Errorf("stdout.reason = %v, want not_configured", payload["reason"])
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty in JSON mode, got:\n%s", stderr.String())
	}
}

func TestAuthLogoutRun_JSONMode_NotLoggedIn_WritesStdoutOnly(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	writeLogoutConfig(t, nil)

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authLogoutRun(&LogoutOptions{Factory: f, JSON: true}); err != nil {
		t.Fatalf("authLogoutRun() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != true {
		t.Errorf("stdout.ok = %v, want true", payload["ok"])
	}
	if payload["loggedOut"] != false {
		t.Errorf("stdout.loggedOut = %v, want false", payload["loggedOut"])
	}
	if payload["reason"] != "not_logged_in" {
		t.Errorf("stdout.reason = %v, want not_logged_in", payload["reason"])
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty in JSON mode, got:\n%s", stderr.String())
	}
}

func TestAuthLogoutRun_JSONMode_Success_WritesStdoutOnly(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	writeLogoutConfig(t, []core.AppUser{{UserOpenId: "ou_user", UserName: "tester"}})
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:      "test-app",
		UserOpenId: "ou_user",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authLogoutRun(&LogoutOptions{Factory: f, JSON: true}); err != nil {
		t.Fatalf("authLogoutRun() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != true {
		t.Errorf("stdout.ok = %v, want true", payload["ok"])
	}
	if payload["loggedOut"] != true {
		t.Errorf("stdout.loggedOut = %v, want true", payload["loggedOut"])
	}
	if _, hasReason := payload["reason"]; hasReason {
		t.Errorf("stdout.reason must be absent on success, got %v", payload["reason"])
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty in JSON mode, got:\n%s", stderr.String())
	}
}

func TestAuthLogoutRun_DefaultMode_KeepsTextOutput(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	writeLogoutConfig(t, []core.AppUser{{UserOpenId: "ou_user", UserName: "tester"}})
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:      "test-app",
		UserOpenId: "ou_user",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("authLogoutRun() error = %v", err)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout must stay empty in default mode, got:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Logged out") {
		t.Errorf("stderr = %q, want success text", stderr.String())
	}
}

func TestAuthLogoutRun_RevokesTokenAndClearsLocalState(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "cli_test",
				AppSecret: core.PlainSecret("secret"),
				Brand:     core.BrandFeishu,
				Users:     []core.AppUser{{UserOpenId: "ou_user", UserName: "tester"}},
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:        "cli_test",
		UserOpenId:   "ou_user",
		AccessToken:  "user-access-token",
		RefreshToken: "user-refresh-token",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthRevoke,
		Body:   map[string]interface{}{"code": 0},
		BodyFilter: func(body []byte) bool {
			values, err := url.ParseQuery(string(body))
			if err != nil {
				return false
			}
			return values.Get("client_id") == "cli_test" &&
				values.Get("client_secret") == "secret" &&
				values.Get("token") == "user-refresh-token" &&
				values.Get("token_type_hint") == "refresh_token"
		},
	})

	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("authLogoutRun() error = %v", err)
	}

	if got := stderr.String(); !strings.Contains(got, "Logged out") {
		t.Fatalf("stderr = %q, want Logged out", got)
	}
	if got := larkauth.GetStoredToken("cli_test", "ou_user"); got != nil {
		t.Fatalf("expected stored token removed, got %#v", got)
	}
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if len(saved.Apps) != 1 || len(saved.Apps[0].Users) != 0 {
		t.Fatalf("expected users cleared, got %#v", saved.Apps)
	}
}

func TestAuthLogoutRun_FallsBackToAccessTokenWhenRefreshTokenMissing(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "cli_test",
				AppSecret: core.PlainSecret("secret"),
				Brand:     core.BrandFeishu,
				Users:     []core.AppUser{{UserOpenId: "ou_user", UserName: "tester"}},
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:       "cli_test",
		UserOpenId:  "ou_user",
		AccessToken: "user-access-token",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthRevoke,
		Body:   map[string]interface{}{"code": 0},
		BodyFilter: func(body []byte) bool {
			values, err := url.ParseQuery(string(body))
			if err != nil {
				return false
			}
			return values.Get("client_id") == "cli_test" &&
				values.Get("client_secret") == "secret" &&
				values.Get("token") == "user-access-token" &&
				values.Get("token_type_hint") == "access_token"
		},
	})

	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("authLogoutRun() error = %v", err)
	}

	if got := stderr.String(); !strings.Contains(got, "Logged out") {
		t.Fatalf("stderr = %q, want Logged out", got)
	}
	if got := larkauth.GetStoredToken("cli_test", "ou_user"); got != nil {
		t.Fatalf("expected stored token removed, got %#v", got)
	}
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if len(saved.Apps) != 1 || len(saved.Apps[0].Users) != 0 {
		t.Fatalf("expected users cleared, got %#v", saved.Apps)
	}
}

func TestAuthLogoutRun_RevokeFailureStillClearsLocalState(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "cli_test",
				AppSecret: core.PlainSecret("secret"),
				Brand:     core.BrandFeishu,
				Users:     []core.AppUser{{UserOpenId: "ou_user", UserName: "tester"}},
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:        "cli_test",
		UserOpenId:   "ou_user",
		AccessToken:  "user-access-token",
		RefreshToken: "user-refresh-token",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthRevoke,
		Status: 500,
		Body:   map[string]interface{}{"error": "server_error"},
	})

	if err := authLogoutRun(&LogoutOptions{Factory: f}); err != nil {
		t.Fatalf("authLogoutRun() error = %v", err)
	}

	gotErr := stderr.String()
	if strings.Contains(gotErr, "failed to revoke token for ou_user") {
		t.Fatalf("stderr = %q, want no revoke warning", gotErr)
	}
	if !strings.Contains(gotErr, "Logged out") {
		t.Fatalf("stderr = %q, want Logged out", gotErr)
	}
	if got := larkauth.GetStoredToken("cli_test", "ou_user"); got != nil {
		t.Fatalf("expected stored token removed, got %#v", got)
	}
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if len(saved.Apps) != 1 || len(saved.Apps[0].Users) != 0 {
		t.Fatalf("expected users cleared, got %#v", saved.Apps)
	}
}
