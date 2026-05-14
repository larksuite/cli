// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/shortcuts/common"
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

func TestNewCmdDoctorPreflight_InvalidFormat(t *testing.T) {
	cfg := &core.CliConfig{
		ProfileName: "default",
		AppID:       "app-1",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	}
	f, _, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tat-token"})

	cmd := NewCmdDoctorPreflight(f)
	cmd.SetArgs([]string{"calendar", "+agenda", "--format", "table"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid format error")
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
	if result.Execution.Command != "lark-cli im +messages-send --as bot" {
		t.Fatalf("execution command = %q, want bot command template", result.Execution.Command)
	}
	if result.Execution.DryRunCommand != "lark-cli im +messages-send --as bot --dry-run" {
		t.Fatalf("dry-run command = %q, want dry-run command template", result.Execution.DryRunCommand)
	}
	if !result.Execution.SupportsDryRun {
		t.Fatal("expected supports_dry_run=true")
	}
}

func TestDoctorPreflight_HighRiskExecutionPlan(t *testing.T) {
	cfg := &core.CliConfig{
		ProfileName: "default",
		AppID:       "app-1",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	}
	f, stdout, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tat-token"})

	cmd := NewCmdDoctor(f)
	cmd.SetArgs([]string{"preflight", "wiki", "+delete-space", "--as", "bot"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result preflightResult
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("failed to parse stdout JSON: %v\n%s", unmarshalErr, stdout.String())
	}
	if result.Execution.Command != "lark-cli wiki +delete-space --as bot --space-id <space-id> --yes" {
		t.Fatalf("execution command = %q, want high-risk command template", result.Execution.Command)
	}
	if result.Execution.DryRunCommand != "lark-cli wiki +delete-space --as bot --space-id <space-id> --dry-run" {
		t.Fatalf("dry-run command = %q, want dry-run command template", result.Execution.DryRunCommand)
	}
	if !result.Execution.RequiresConfirmation {
		t.Fatal("expected requires_confirmation=true")
	}
	if len(result.Execution.Flags) != 1 {
		t.Fatalf("execution flags = %+v, want one required flag", result.Execution.Flags)
	}
	flag := result.Execution.Flags[0]
	if flag.Name != "space-id" || !flag.Required || flag.Type != "string" {
		t.Fatalf("execution flag = %+v, want required string space-id", flag)
	}
}

func TestResolveTargetShortcutErrors(t *testing.T) {
	if _, err := resolveTargetShortcut("no-service", "+agenda"); err == nil {
		t.Fatal("expected error for unknown service")
	}
	if _, err := resolveTargetShortcut("calendar", "+not-found"); err == nil {
		t.Fatal("expected error for unknown shortcut command")
	}
}

func TestResolvePreflightIdentitySources(t *testing.T) {
	t.Run("invalid as", func(t *testing.T) {
		cfg := &core.CliConfig{AppID: "app-1", AppSecret: "secret", Brand: core.BrandFeishu}
		f, _, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tok"})
		_, _, err := resolvePreflightIdentity(&DoctorPreflightOptions{
			Factory:     f,
			Ctx:         context.Background(),
			RequestedAs: "demo",
		}, cfg)
		if err == nil {
			t.Fatal("expected invalid as error")
		}
	})

	t.Run("explicit as", func(t *testing.T) {
		cfg := &core.CliConfig{AppID: "app-1", AppSecret: "secret", Brand: core.BrandFeishu}
		f, _, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tok"})
		as, source, err := resolvePreflightIdentity(&DoctorPreflightOptions{
			Factory:     f,
			Ctx:         context.Background(),
			RequestedAs: "bot",
		}, cfg)
		if err != nil {
			t.Fatalf("resolvePreflightIdentity() error = %v", err)
		}
		if as != core.AsBot || source != "explicit_as" {
			t.Fatalf("got (%s,%s), want (bot,explicit_as)", as, source)
		}
	})

	t.Run("strict mode", func(t *testing.T) {
		cfg := &core.CliConfig{
			AppID:               "app-1",
			AppSecret:           "secret",
			Brand:               core.BrandFeishu,
			SupportedIdentities: uint8(extcred.SupportsBot),
		}
		f, _, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tok"})
		as, source, err := resolvePreflightIdentity(&DoctorPreflightOptions{
			Factory:     f,
			Ctx:         context.Background(),
			RequestedAs: "auto",
		}, cfg)
		if err != nil {
			t.Fatalf("resolvePreflightIdentity() error = %v", err)
		}
		if as != core.AsBot || source != "strict_mode" {
			t.Fatalf("got (%s,%s), want (bot,strict_mode)", as, source)
		}
	})

	t.Run("default as", func(t *testing.T) {
		cfg := &core.CliConfig{
			AppID:     "app-1",
			AppSecret: "secret",
			Brand:     core.BrandFeishu,
			DefaultAs: core.AsUser,
		}
		f, _, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tok"})
		as, source, err := resolvePreflightIdentity(&DoctorPreflightOptions{
			Factory:     f,
			Ctx:         context.Background(),
			RequestedAs: "auto",
		}, cfg)
		if err != nil {
			t.Fatalf("resolvePreflightIdentity() error = %v", err)
		}
		if as != core.AsUser || source != "default_as" {
			t.Fatalf("got (%s,%s), want (user,default_as)", as, source)
		}
	})
}

func TestEvaluateUserTokenReadinessBranches(t *testing.T) {
	cfg := &core.CliConfig{
		AppID:      "app-1",
		AppSecret:  "secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_1",
		UserName:   "alice",
	}

	t.Run("no user logged in", func(t *testing.T) {
		f, _, _ := newPreflightFactory(t, &core.CliConfig{
			AppID:     "app-1",
			AppSecret: "secret",
			Brand:     core.BrandFeishu,
		}, &credential.TokenResult{Token: "tok"})
		_, check, action, ok := evaluateUserTokenReadiness(&DoctorPreflightOptions{Factory: f, Ctx: context.Background()}, &core.CliConfig{
			AppID:     "app-1",
			AppSecret: "secret",
			Brand:     core.BrandFeishu,
		}, []string{"calendar:calendar.event:read"})
		if ok || check.Status != "fail" || action == nil {
			t.Fatalf("got ok=%v check=%+v action=%+v", ok, check, action)
		}
	})

	t.Run("token resolve error", func(t *testing.T) {
		f, _, _ := newPreflightFactory(t, cfg, nil)
		f.Credential = credential.NewCredentialProvider(nil, &preflightAccountResolver{cfg: cfg}, &preflightTokenResolver{err: errors.New("boom")}, nil)
		_, check, action, ok := evaluateUserTokenReadiness(&DoctorPreflightOptions{Factory: f, Ctx: context.Background()}, cfg, []string{"calendar:calendar.event:read"})
		if ok || check.Status != "fail" || action == nil {
			t.Fatalf("got ok=%v check=%+v action=%+v", ok, check, action)
		}
	})

	t.Run("expired stored token", func(t *testing.T) {
		restorePreflightTokenFuncs(t)
		preflightGetStoredToken = func(appID, openID string) *internalauth.StoredUAToken {
			return &internalauth.StoredUAToken{AppId: appID, UserOpenId: openID}
		}
		preflightTokenStatus = func(token *internalauth.StoredUAToken) string { return "expired" }
		f, _, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tok"})
		_, check, action, ok := evaluateUserTokenReadiness(&DoctorPreflightOptions{Factory: f, Ctx: context.Background()}, cfg, []string{"calendar:calendar.event:read"})
		if ok || check.Status != "fail" || action == nil {
			t.Fatalf("got ok=%v check=%+v action=%+v", ok, check, action)
		}
	})

	t.Run("needs refresh", func(t *testing.T) {
		restorePreflightTokenFuncs(t)
		preflightGetStoredToken = func(appID, openID string) *internalauth.StoredUAToken {
			return &internalauth.StoredUAToken{AppId: appID, UserOpenId: openID}
		}
		preflightTokenStatus = func(token *internalauth.StoredUAToken) string { return "needs_refresh" }
		f, _, _ := newPreflightFactory(t, cfg, &credential.TokenResult{Token: "tok"})
		_, check, action, ok := evaluateUserTokenReadiness(&DoctorPreflightOptions{Factory: f, Ctx: context.Background()}, cfg, []string{"calendar:calendar.event:read"})
		if !ok || check.Status != "pass" || action != nil {
			t.Fatalf("got ok=%v check=%+v action=%+v", ok, check, action)
		}
	})
}

func TestEvaluateScopeReadinessBranches(t *testing.T) {
	check, action := evaluateScopeReadiness(nil, &credential.TokenResult{Token: "tok"})
	if check.Status != "pass" || action != nil {
		t.Fatalf("no-scope branch = %+v %+v", check, action)
	}

	check, action = evaluateScopeReadiness([]string{"calendar:calendar.event:read"}, &credential.TokenResult{Token: "tok"})
	if check.Status != "unknown" || action != nil {
		t.Fatalf("unknown-scope branch = %+v %+v", check, action)
	}

	check, action = evaluateScopeReadiness([]string{"calendar:calendar.event:read"}, &credential.TokenResult{
		Token:  "tok",
		Scopes: "calendar:calendar.event:read",
	})
	if check.Status != "pass" || action != nil {
		t.Fatalf("granted-scope branch = %+v %+v", check, action)
	}
}

func TestWritePreflightResultPrettyAndConfigFailure(t *testing.T) {
	t.Run("pretty output", func(t *testing.T) {
		var buf bytes.Buffer
		writePreflightResult(&buf, preflightResult{
			OK:        true,
			Ready:     true,
			Workspace: "local",
			Target: preflightTarget{
				Service: "calendar",
				Command: "+agenda",
				Scopes:  []string{"calendar:calendar.event:read"},
			},
			Identity: preflightIdentity{
				Requested: "auto",
				Resolved:  "user",
				Source:    "default_as",
			},
			Execution: preflightExecution{
				Command:        "lark-cli calendar +agenda --as user",
				DryRunCommand:  "lark-cli calendar +agenda --as user --dry-run",
				SupportsDryRun: true,
				Flags: []preflightExecutionFlag{{
					Name:        "calendar-id",
					Description: "calendar id",
				}},
			},
			Checks: []preflightCheck{{
				Name:    "config_ready",
				Status:  "pass",
				Message: "ok",
			}},
			NextActions: []preflightAction{{
				Type:    "dry_run",
				Command: "lark-cli calendar +agenda --dry-run",
				Reason:  "preview first",
			}},
		}, "pretty")
		out := buf.String()
		for _, want := range []string{"Shortcut Preflight: READY", "Flags:", "Next Actions:", "Dry Run:"} {
			if !strings.Contains(out, want) {
				t.Fatalf("pretty output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("agent workspace config failure", func(t *testing.T) {
		prev := core.CurrentWorkspace()
		core.SetCurrentWorkspace(core.WorkspaceHermes)
		t.Cleanup(func() { core.SetCurrentWorkspace(prev) })
		result := buildConfigFailureResult(preflightTarget{
			Service: "calendar",
			Command: "+agenda",
		}, preflightIdentity{Requested: "auto"}, &core.ConfigError{
			Code:    2,
			Type:    "hermes",
			Message: "hermes context detected but lark-cli is not bound to it",
			Hint:    "read `lark-cli config bind --help`",
		})
		if result.NextActions[0].Command != "lark-cli config bind --help" {
			t.Fatalf("next action = %+v, want bind help", result.NextActions)
		}
	})
}

func TestHelperFunctions(t *testing.T) {
	if got := shortcutRisk(nil); got != "read" {
		t.Fatalf("shortcutRisk(nil) = %q, want read", got)
	}
	if got := shortcutAuthTypes(nil); len(got) != 1 || got[0] != "user" {
		t.Fatalf("shortcutAuthTypes(nil) = %#v, want [user]", got)
	}
	if got := buildAuthLoginCommand(nil); got != "lark-cli auth login --help" {
		t.Fatalf("buildAuthLoginCommand(nil) = %q", got)
	}
	if got := normalizedRequestedIdentity(""); got != "auto" {
		t.Fatalf("normalizedRequestedIdentity(\"\") = %q", got)
	}
	if got := normalizedFlagType(""); got != "string" {
		t.Fatalf("normalizedFlagType(\"\") = %q", got)
	}
	if got := normalizedFlagType("bool"); got != "bool" {
		t.Fatalf("normalizedFlagType(\"bool\") = %q", got)
	}
	if got := flagPlaceholder(common.Flag{Name: "apply", Type: "bool"}); got != "true" {
		t.Fatalf("flagPlaceholder(bool) = %q", got)
	}
	if isPreflightReady([]preflightCheck{{Status: "fail", Blocking: true}}) {
		t.Fatal("expected ready=false for blocking fail")
	}
}

func TestAppendRequiredFlagTemplate_BoolFlag(t *testing.T) {
	args := appendRequiredFlagTemplate([]string{"lark-cli", "demo", "+run"}, common.Flag{
		Name:     "apply",
		Type:     "bool",
		Required: true,
	})
	want := []string{"lark-cli", "demo", "+run", "--apply"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}

func TestBuildPreflightExecutionHelpers(t *testing.T) {
	flags := buildPreflightExecutionFlags([]common.Flag{
		{Name: "title", Desc: "title"},
		{Name: "hidden", Hidden: true},
	})
	if len(flags) != 1 || flags[0].Type != "string" {
		t.Fatalf("buildPreflightExecutionFlags() = %+v", flags)
	}

	notes := buildPreflightExecutionNotes(&common.Shortcut{
		Risk: "write",
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			return common.NewDryRunAPI()
		},
	})
	if len(notes) == 0 {
		t.Fatal("expected notes for write shortcut")
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

func restorePreflightTokenFuncs(t *testing.T) {
	t.Helper()
	origGet := preflightGetStoredToken
	origStatus := preflightTokenStatus
	t.Cleanup(func() {
		preflightGetStoredToken = origGet
		preflightTokenStatus = origStatus
	})
}
