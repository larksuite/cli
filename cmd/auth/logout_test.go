// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"strings"
	"testing"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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
