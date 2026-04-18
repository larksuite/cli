// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// assertExitError checks the full structured error in one assertion.
func assertExitError(t *testing.T, err error, wantCode int, wantDetail output.ErrDetail) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *output.ExitError; error = %v", err, err)
	}
	if exitErr.Code != wantCode {
		t.Errorf("exit code = %d, want %d", exitErr.Code, wantCode)
	}
	if exitErr.Detail == nil {
		t.Fatal("expected non-nil error detail")
	}
	if !reflect.DeepEqual(*exitErr.Detail, wantDetail) {
		t.Errorf("error detail mismatch:\n  got:  %+v\n  want: %+v", *exitErr.Detail, wantDetail)
	}
}

// saveWorkspace saves the current workspace and returns a cleanup func to restore it.
// Must be called at the start of any test that may trigger configBindRun (which sets workspace).
func saveWorkspace(t *testing.T) {
	t.Helper()
	orig := core.CurrentWorkspace()
	t.Cleanup(func() { core.SetCurrentWorkspace(orig) })
}

// ── Command flag parsing tests (aligned with config_test.go pattern) ──

func TestConfigBindCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *BindOptions
	cmd := NewCmdConfigBind(f, func(opts *BindOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--source", "openclaw", "--app-id", "cli_test", "--strict-mode", "bot", "--default-as", "user", "--lang", "en", "--force"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Source != "openclaw" {
		t.Errorf("Source = %q, want %q", gotOpts.Source, "openclaw")
	}
	if gotOpts.AppID != "cli_test" {
		t.Errorf("AppID = %q, want %q", gotOpts.AppID, "cli_test")
	}
	if gotOpts.StrictMode != "bot" {
		t.Errorf("StrictMode = %q, want %q", gotOpts.StrictMode, "bot")
	}
	if gotOpts.DefaultAs != "user" {
		t.Errorf("DefaultAs = %q, want %q", gotOpts.DefaultAs, "user")
	}
	if gotOpts.Lang != "en" {
		t.Errorf("Lang = %q, want %q", gotOpts.Lang, "en")
	}
	if !gotOpts.Force {
		t.Error("expected Force=true")
	}
	if !gotOpts.langExplicit {
		t.Error("expected langExplicit=true when --lang is passed")
	}
}

func TestConfigBindCmd_LangDefault(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *BindOptions
	cmd := NewCmdConfigBind(f, func(opts *BindOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--source", "hermes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Lang != "zh" {
		t.Errorf("Lang = %q, want default %q", gotOpts.Lang, "zh")
	}
	if gotOpts.langExplicit {
		t.Error("expected langExplicit=false when --lang not passed")
	}
}

// ── Run function tests (aligned with TestConfigShowRun pattern) ──

func TestConfigBindRun_InvalidSource(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "invalid"})
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "validation",
		Message: `invalid --source "invalid"; valid values: openclaw, hermes`,
	})
}

func TestConfigBindRun_MissingSourceNonTTY(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// TestFactory has IsTerminal=false by default
	err := configBindRun(&BindOptions{Factory: f, Source: ""})
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "bind",
		Message: "--source is required (openclaw or hermes)",
		Hint:    "lark-cli config bind --source openclaw",
	})
}

func TestConfigBindRun_ConflictWithoutForce(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	// Pre-create hermes workspace config
	hermesDir := filepath.Join(configDir, "hermes")
	if err := os.MkdirAll(hermesDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hermesDir, "config.json"), []byte(`{"apps":[{"appId":"old"}]}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Point HERMES_HOME to a valid .env
	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_new\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes", Force: false})
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "bind",
		Message: `workspace "hermes" already bound at ` + filepath.Join(configDir, "hermes", "config.json"),
		Hint:    "pass --force to replace, or run 'lark-cli config bind' (no flags) for interactive mode",
	})
}

func TestConfigBindRun_HermesMissingEnvFile(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	hermesHome := filepath.Join(t.TempDir(), "nonexistent")
	t.Setenv("HERMES_HOME", hermesHome)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	envPath := filepath.Join(hermesHome, ".env")
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "hermes",
		Message: "failed to read Hermes config: open " + envPath + ": no such file or directory",
		Hint:    "verify Hermes is installed and configured at " + envPath,
	})
}

func TestConfigBindRun_OpenClawMissingFile(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	openclawHome := filepath.Join(t.TempDir(), "nonexistent")
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	configPath := filepath.Join(openclawHome, ".openclaw", "openclaw.json")
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: "cannot read " + configPath + ": open " + configPath + ": no such file or directory",
		Hint:    "verify OpenClaw is installed and configured",
	})
}

func TestConfigShowRun_WorkspaceField(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	core.SetCurrentWorkspace(core.WorkspaceLocal)

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:     "cli_local_test",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("save: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err != nil {
		t.Fatalf("configShowRun error: %v", err)
	}
	// If we get here without error, show succeeded.
	// Workspace field in JSON output is verified by e2e tests (real binary output).
}

func TestConfigShowRun_AgentWorkspaceNotBound(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err == nil {
		t.Fatal("expected error for unbound workspace")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *output.ExitError", err)
	}
	// Should suggest config bind, not config init
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: "openclaw context detected but lark-cli not bound to openclaw workspace",
		Hint:    "run: lark-cli config bind --source openclaw",
	})
}

// ── Helper function tests (dotenv, brand, path resolution) ──

func TestReadDotenv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	content := "# Hermes config\nFEISHU_APP_ID=cli_abc123\nFEISHU_APP_SECRET=supersecret\nFEISHU_DOMAIN=lark\n\nFEISHU_CONNECTION_MODE=websocket\n"
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	got, err := readDotenv(envPath)
	if err != nil {
		t.Fatalf("readDotenv() error: %v", err)
	}

	checks := map[string]string{
		"FEISHU_APP_ID":          "cli_abc123",
		"FEISHU_APP_SECRET":      "supersecret",
		"FEISHU_DOMAIN":          "lark",
		"FEISHU_CONNECTION_MODE": "websocket",
	}
	for key, want := range checks {
		if got[key] != want {
			t.Errorf("key %q = %q, want %q", key, got[key], want)
		}
	}
}

func TestReadDotenv_FileNotFound(t *testing.T) {
	_, err := readDotenv("/nonexistent/path/.env")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadDotenv_ValueWithEquals(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := `DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=require`
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	got, err := readDotenv(envPath)
	if err != nil {
		t.Fatalf("readDotenv() error: %v", err)
	}
	want := "postgres://user:pass@host:5432/db?sslmode=require"
	if got["DATABASE_URL"] != want {
		t.Errorf("DATABASE_URL = %q, want %q", got["DATABASE_URL"], want)
	}
}

func TestNormalizeBrand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "feishu"},
		{"feishu", "feishu"},
		{"lark", "lark"},
		{"LARK", "lark"},
		{" lark ", "lark"},
		{"Lark", "lark"},
	}
	for _, tt := range tests {
		if got := normalizeBrand(tt.input); got != tt.want {
			t.Errorf("normalizeBrand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveOpenClawConfigPath_Overrides(t *testing.T) {
	t.Run("OPENCLAW_CONFIG_PATH wins", func(t *testing.T) {
		custom := filepath.Join(t.TempDir(), "custom.json")
		t.Setenv("OPENCLAW_CONFIG_PATH", custom)
		t.Setenv("OPENCLAW_STATE_DIR", "")
		t.Setenv("OPENCLAW_HOME", "")
		if got := resolveOpenClawConfigPath(); got != custom {
			t.Errorf("got %q, want %q", got, custom)
		}
	})

	t.Run("OPENCLAW_STATE_DIR", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("OPENCLAW_CONFIG_PATH", "")
		t.Setenv("OPENCLAW_STATE_DIR", dir)
		t.Setenv("OPENCLAW_HOME", "")
		want := filepath.Join(dir, "openclaw.json")
		if got := resolveOpenClawConfigPath(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("OPENCLAW_HOME", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("OPENCLAW_CONFIG_PATH", "")
		t.Setenv("OPENCLAW_STATE_DIR", "")
		t.Setenv("OPENCLAW_HOME", dir)
		want := filepath.Join(dir, ".openclaw", "openclaw.json")
		if got := resolveOpenClawConfigPath(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestResolveHermesEnvPath_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HERMES_HOME", tmp)
	want := filepath.Join(tmp, ".env")
	if got := resolveHermesEnvPath(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── Success path tests (Hermes bind flow) ──

func TestConfigBindRun_HermesSuccess(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	envContent := "FEISHU_APP_ID=cli_hermes_abc\nFEISHU_APP_SECRET=hermes_secret_123\nFEISHU_DOMAIN=lark\n"
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte(envContent), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes", Lang: "en"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("ok = %v, want true", result["ok"])
	}
	if result["workspace"] != "hermes" {
		t.Errorf("workspace = %v, want %q", result["workspace"], "hermes")
	}
	if result["app_id"] != "cli_hermes_abc" {
		t.Errorf("app_id = %v, want %q", result["app_id"], "cli_hermes_abc")
	}

	targetPath := filepath.Join(configDir, "hermes", "config.json")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var multi core.MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		t.Fatalf("unmarshal config.json: %v", err)
	}
	if len(multi.Apps) != 1 {
		t.Fatalf("apps count = %d, want 1", len(multi.Apps))
	}
	if multi.Apps[0].AppId != "cli_hermes_abc" {
		t.Errorf("appId = %q, want %q", multi.Apps[0].AppId, "cli_hermes_abc")
	}
	if multi.Apps[0].Brand != core.BrandLark {
		t.Errorf("brand = %q, want %q", multi.Apps[0].Brand, core.BrandLark)
	}
}

func TestConfigBindRun_OpenClawSuccess_SingleAccount(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"appId":"cli_oc_123","appSecret":"oc_secret_456","brand":"feishu"}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", Lang: "zh"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("ok = %v, want true", result["ok"])
	}
	if result["workspace"] != "openclaw" {
		t.Errorf("workspace = %v, want %q", result["workspace"], "openclaw")
	}
	if result["app_id"] != "cli_oc_123" {
		t.Errorf("app_id = %v, want %q", result["app_id"], "cli_oc_123")
	}
}

func TestConfigBindRun_OpenClawMultiAccount_WithAppID(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{
		"channels":{"feishu":{
			"accounts":{
				"work":{"appId":"cli_work_111","appSecret":"secret_work","brand":"feishu"},
				"personal":{"appId":"cli_personal_222","appSecret":"secret_personal","brand":"lark"}
			}
		}}
	}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", AppID: "cli_personal_222"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["app_id"] != "cli_personal_222" {
		t.Errorf("app_id = %v, want %q", result["app_id"], "cli_personal_222")
	}
}

func TestConfigBindRun_OpenClawMultiAccount_MissingAppID(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{
		"channels":{"feishu":{
			"accounts":{
				"work":{"appId":"cli_work_111","appSecret":"secret_work"},
				"personal":{"appId":"cli_personal_222","appSecret":"secret_personal"}
			}
		}}
	}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	if err == nil {
		t.Fatal("expected error for multi-account without --app-id, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *output.ExitError", err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
}

// TestConfigBindRun_OpenClawMultiAccount_TTYFlagMode asserts the end-to-end
// contract: passing --source on a real terminal is flag-mode. With multiple
// candidates and no --app-id, the command must error with the candidate list
// instead of opening an interactive prompt just because stdin is a TTY.
func TestConfigBindRun_OpenClawMultiAccount_TTYFlagMode(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{
		"channels":{"feishu":{
			"accounts":{
				"work":{"appId":"cli_work_111","appSecret":"secret_work"},
				"personal":{"appId":"cli_personal_222","appSecret":"secret_personal"}
			}
		}}
	}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// Simulate a real terminal. Because --source is explicit, opts.IsTUI is
	// still false, so selectCandidate must refuse the multi-candidate case
	// with a validation error rather than opening the huh prompt.
	f.IOStreams.IsTerminal = true

	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})

	// The hint's candidate list comes from openclaw.ListCandidateApps, which
	// iterates a map — ordering is non-deterministic. DeepEqual inline against
	// each accepted variant so every ErrDetail field (Type, Code, Message,
	// Hint, ConsoleURL, Detail, and any future addition) is still compared.
	base := output.ErrDetail{
		Type:    "openclaw",
		Message: "multiple accounts in openclaw.json; pass --app-id <id>",
	}
	wantWorkFirst := base
	wantWorkFirst.Hint = "available app IDs:\n  cli_work_111 (work)\n  cli_personal_222 (personal)"
	wantPersonalFirst := base
	wantPersonalFirst.Hint = "available app IDs:\n  cli_personal_222 (personal)\n  cli_work_111 (work)"

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *output.ExitError; err = %v", err, err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
	if exitErr.Detail == nil {
		t.Fatal("expected non-nil error detail")
	}
	if !reflect.DeepEqual(*exitErr.Detail, wantWorkFirst) &&
		!reflect.DeepEqual(*exitErr.Detail, wantPersonalFirst) {
		t.Errorf("error detail did not match any accepted variant:\n  got:  %+v\n  want: %+v OR %+v",
			*exitErr.Detail, wantWorkFirst, wantPersonalFirst)
	}
}

func TestConfigBindRun_OpenClawMultiAccount_WrongAppID(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"appId":"cli_only_one","appSecret":"secret_only"}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", AppID: "nonexistent"})
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: `--app-id "nonexistent" not found in openclaw.json`,
		Hint:    "available app IDs:\n  cli_only_one",
	})
}

func TestConfigBindRun_InvalidStrictMode(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes", StrictMode: "invalid"})
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "validation",
		Message: `invalid --strict-mode "invalid"; valid values: bot, user, off`,
	})
}

func TestConfigBindRun_InvalidDefaultAs(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes", DefaultAs: "nobody"})
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "validation",
		Message: `invalid --default-as "nobody"; valid values: user, bot, auto`,
	})
}

func TestConfigBindRun_StrictModeAndDefaultAs_Applied(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{
		Factory:    f,
		Source:     "hermes",
		StrictMode: "bot",
		DefaultAs:  "user",
		Lang:       "en",
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	targetPath := filepath.Join(configDir, "hermes", "config.json")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var multi core.MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		t.Fatalf("unmarshal config.json: %v", err)
	}
	if multi.Apps[0].StrictMode == nil {
		t.Fatal("StrictMode should be set")
	}
	if *multi.Apps[0].StrictMode != core.StrictMode("bot") {
		t.Errorf("StrictMode = %q, want %q", *multi.Apps[0].StrictMode, "bot")
	}
	if multi.Apps[0].DefaultAs != core.Identity("user") {
		t.Errorf("DefaultAs = %q, want %q", multi.Apps[0].DefaultAs, "user")
	}
}

func TestConfigBindRun_ForceOverwrite(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesDir := filepath.Join(configDir, "hermes")
	if err := os.MkdirAll(hermesDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hermesDir, "config.json"), []byte(`{"apps":[{"appId":"old_app"}]}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_new_app\nFEISHU_APP_SECRET=new_secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes", Force: true})
	if err != nil {
		t.Fatalf("expected success with --force, got error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["app_id"] != "cli_new_app" {
		t.Errorf("app_id = %v, want %q", result["app_id"], "cli_new_app")
	}
}

func TestConfigBindRun_HermesMissingAppID(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_SECRET=secret_only\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	envPath := filepath.Join(hermesHome, ".env")
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "hermes",
		Message: "FEISHU_APP_ID not found in " + envPath,
		Hint:    "run 'hermes setup' to configure Feishu credentials",
	})
}

func TestConfigBindRun_HermesMissingAppSecret(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	envPath := filepath.Join(hermesHome, ".env")
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "hermes",
		Message: "FEISHU_APP_SECRET not found in " + envPath,
		Hint:    "run 'hermes setup' to configure Feishu credentials",
	})
}

func TestConfigBindRun_OpenClawMissingFeishu(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(`{"channels":{}}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: "openclaw.json missing channels.feishu section",
		Hint:    "configure Feishu in OpenClaw first",
	})
}

func TestConfigBindRun_OpenClawEmptyAppSecret(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"appId":"cli_no_secret","appSecret":""}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	openclawPath := filepath.Join(openclawDir, "openclaw.json")
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: "appSecret is empty for app cli_no_secret in " + openclawPath,
		Hint:    "configure channels.feishu.appSecret in openclaw.json",
	})
}

func TestConfigBindRun_OpenClawEnvTemplate(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")
	t.Setenv("MY_OC_SECRET", "resolved_env_secret")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"appId":"cli_env_test","appSecret":"${MY_OC_SECRET}","brand":"lark"}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["app_id"] != "cli_env_test" {
		t.Errorf("app_id = %v, want %q", result["app_id"], "cli_env_test")
	}
}

func TestConfigBindRun_OpenClawDisabledAccount(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"accounts":{"work":{"appId":"cli_disabled","appSecret":"secret","enabled":false}}}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: "no Feishu app configured in openclaw.json",
		Hint:    "configure channels.feishu.appId in openclaw.json",
	})
}

// ── getBindMsg tests ──

func TestGetBindMsg_Zh(t *testing.T) {
	msg := getBindMsg("zh")
	if msg.SelectSource != "选择绑定来源" {
		t.Errorf("zh SelectSource = %q, want %q", msg.SelectSource, "选择绑定来源")
	}
	if msg.SelectStrictMode != "选择严格模式" {
		t.Errorf("zh SelectStrictMode = %q, want %q", msg.SelectStrictMode, "选择严格模式")
	}
	if msg.SelectDefaultAs != "选择默认身份" {
		t.Errorf("zh SelectDefaultAs = %q, want %q", msg.SelectDefaultAs, "选择默认身份")
	}
}

func TestGetBindMsg_En(t *testing.T) {
	msg := getBindMsg("en")
	if msg.SelectSource != "Select Agent source to bind" {
		t.Errorf("en SelectSource = %q, want %q", msg.SelectSource, "Select Agent source to bind")
	}
	if msg.StrictModeOff != "off — no restriction (recommended)" {
		t.Errorf("en StrictModeOff = %q, want %q", msg.StrictModeOff, "off — no restriction (recommended)")
	}
	if msg.DefaultAsAuto != "auto — infer automatically (default)" {
		t.Errorf("en DefaultAsAuto = %q, want %q", msg.DefaultAsAuto, "auto — infer automatically (default)")
	}
}

func TestGetBindMsg_UnknownLang_DefaultsToZh(t *testing.T) {
	msg := getBindMsg("fr")
	if msg.SelectSource != "选择绑定来源" {
		t.Errorf("fr (default) SelectSource = %q, want %q", msg.SelectSource, "选择绑定来源")
	}
}

// ── Resolve path edge case tests ──

func TestResolveOpenClawConfigPath_LegacyFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")
	t.Setenv("OPENCLAW_HOME", home)

	legacyDir := filepath.Join(home, ".clawdbot")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacyFile := filepath.Join(legacyDir, "clawdbot.json")
	if err := os.WriteFile(legacyFile, []byte(`{}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := resolveOpenClawConfigPath()
	if got != legacyFile {
		t.Errorf("got %q, want legacy fallback %q", got, legacyFile)
	}
}

func TestResolveOpenClawConfigPath_DefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")
	t.Setenv("OPENCLAW_HOME", home)

	want := filepath.Join(home, ".openclaw", "openclaw.json")
	got := resolveOpenClawConfigPath()
	if got != want {
		t.Errorf("got %q, want default %q", got, want)
	}
}

// ── cleanupKeychainFromData ──

func TestCleanupKeychainFromData_InvalidJSON(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// Should not panic on invalid JSON
	cleanupKeychainFromData(f.Keychain, []byte("not json"))
}

func TestCleanupKeychainFromData_ValidConfig(t *testing.T) {
	configData := []byte(`{"apps":[{"appId":"test_app","appSecret":{"ref":{"source":"keychain","id":"test_key"}}}]}`)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// Should not panic — noopKeychain ignores Remove calls
	cleanupKeychainFromData(f.Keychain, configData)
}
