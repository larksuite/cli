// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

type preflightAccountResolver struct {
	cfg *core.CliConfig
}

func (r *preflightAccountResolver) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	return credential.AccountFromCliConfig(r.cfg), nil
}

type preflightTokenResolver struct {
	result *credential.TokenResult
	err    error
}

func (r *preflightTokenResolver) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	return r.result, r.err
}

func TestDoctorPreflight_NotConfiguredLocal(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	f := cmdutil.NewDefault(cmdutil.NewIOStreams(&bytes.Buffer{}, stdout, stderr), cmdutil.InvocationContext{})

	cmd := NewCmdDoctor(f)
	cmd.SetArgs([]string{"preflight", "calendar", "+agenda"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-nil error for not-ready preflight")
	}

	var result preflightResult
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("failed to parse stdout JSON: %v\n%s", unmarshalErr, stdout.String())
	}
	if result.Ready {
		t.Fatal("expected ready=false when config is missing")
	}
	if got := result.Checks[0].Name; got != "config_ready" {
		t.Fatalf("first check = %q, want config_ready", got)
	}
	if len(result.NextActions) == 0 || result.NextActions[0].Command != "lark-cli config init --new" {
		t.Fatalf("next action = %+v, want config init", result.NextActions)
	}
}

func TestDoctorPreflight_UserMissingScopes(t *testing.T) {
	cfg := &core.CliConfig{
		ProfileName: "default",
		AppID:       "app-1",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
		UserOpenId:  "ou_123",
		UserName:    "Alice",
	}
	f, stdout, _ := newPreflightFactory(t, cfg, &credential.TokenResult{
		Token:  "uat-token",
		Scopes: "calendar:calendar:read",
	})

	cmd := NewCmdDoctor(f)
	cmd.SetArgs([]string{"preflight", "calendar", "+agenda", "--as", "user"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-nil error for missing scopes")
	}

	var result preflightResult
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("failed to parse stdout JSON: %v\n%s", unmarshalErr, stdout.String())
	}
	if result.Ready {
		t.Fatal("expected ready=false when scopes are missing")
	}
	scopeCheck := findPreflightCheck(t, result.Checks, "scope_ready")
	if scopeCheck.Status != "fail" {
		t.Fatalf("scope_ready status = %q, want fail", scopeCheck.Status)
	}
	if len(result.NextActions) == 0 || result.NextActions[0].Type != "auth_login" {
		t.Fatalf("next actions = %+v, want auth_login", result.NextActions)
	}
}

func TestDoctorPreflight_StrictModeConflict(t *testing.T) {
	cfg := &core.CliConfig{
		ProfileName:         "default",
		AppID:               "app-1",
		AppSecret:           "secret",
		Brand:               core.BrandFeishu,
		SupportedIdentities: uint8(extcred.SupportsBot),
	}
	f, stdout, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tat-token"})

	cmd := NewCmdDoctor(f)
	cmd.SetArgs([]string{"preflight", "calendar", "+agenda", "--as", "user"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected non-nil error for strict-mode conflict")
	}

	var result preflightResult
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("failed to parse stdout JSON: %v\n%s", unmarshalErr, stdout.String())
	}
	strictCheck := findPreflightCheck(t, result.Checks, "strict_mode")
	if strictCheck.Status != "fail" {
		t.Fatalf("strict_mode status = %q, want fail", strictCheck.Status)
	}
	if len(result.NextActions) == 0 || result.NextActions[0].Command != "lark-cli config strict-mode --help" {
		t.Fatalf("next actions = %+v, want strict-mode help", result.NextActions)
	}
}

func TestDoctorPreflight_WriteShortcutWarnsDryRun(t *testing.T) {
	cfg := &core.CliConfig{
		ProfileName: "default",
		AppID:       "app-1",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	}
	f, stdout, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tat-token"})

	cmd := NewCmdDoctor(f)
	cmd.SetArgs([]string{"preflight", "im", "+messages-send", "--as", "bot"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result preflightResult
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("failed to parse stdout JSON: %v\n%s", unmarshalErr, stdout.String())
	}
	if !result.Ready {
		t.Fatal("expected ready=true for bot write shortcut")
	}
	riskCheck := findPreflightCheck(t, result.Checks, "risk")
	if riskCheck.Status != "warn" {
		t.Fatalf("risk status = %q, want warn", riskCheck.Status)
	}
	foundDryRun := false
	for _, action := range result.NextActions {
		if action.Type == "dry_run" {
			foundDryRun = true
			break
		}
	}
	if !foundDryRun {
		t.Fatalf("next actions = %+v, want dry_run action", result.NextActions)
	}
}

func newPreflightFactory(t *testing.T, cfg *core.CliConfig, token *credential.TokenResult) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	multi := &core.MultiAppConfig{
		CurrentApp: cfg.ProfileName,
		Apps: []core.AppConfig{{
			Name:      cfg.ProfileName,
			AppId:     cfg.AppID,
			AppSecret: core.PlainSecret(cfg.AppSecret),
			Brand:     cfg.Brand,
			DefaultAs: cfg.DefaultAs,
			Users:     []core.AppUser{{UserOpenId: cfg.UserOpenId, UserName: cfg.UserName}},
		}},
	}
	if cfg.ProfileName == "" {
		multi.CurrentApp = ""
		multi.Apps[0].Name = ""
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	f := cmdutil.NewDefault(cmdutil.NewIOStreams(&bytes.Buffer{}, stdout, stderr), cmdutil.InvocationContext{Profile: cfg.ProfileName})
	f.Credential = credential.NewCredentialProvider(nil, &preflightAccountResolver{cfg: cfg}, &preflightTokenResolver{result: token}, nil)
	return f, stdout, stderr
}

func findPreflightCheck(t *testing.T, checks []preflightCheck, name string) preflightCheck {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("check %q not found in %+v", name, checks)
	return preflightCheck{}
}
