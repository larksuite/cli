// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"github.com/larksuite/cli/brand"
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/secret"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/output"
)

func TestExecuteProfileSelectorPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name        string
		envProfile  string
		args        []string
		wantProfile string
		wantSource  string
	}{
		{"environment selects profile", "session", []string{"config", "show"}, "session", "environment"},
		{"flag overrides environment", "session", []string{"config", "show", "--profile", "default"}, "default", "flag"},
		{"empty flag uses persisted default", "session", []string{"config", "show", "--profile="}, "default", "config"},
		{"empty environment uses persisted default", "", []string{"config", "show"}, "default", "config"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpHome(t)
			platform.ResetForTesting()
			t.Cleanup(platform.ResetForTesting)
			t.Setenv(envvars.CliProfile, tc.envProfile)
			t.Setenv(envvars.CliAppID, "")
			t.Setenv(envvars.CliAppSecret, "")
			t.Setenv(envvars.CliUserAccessToken, "")
			t.Setenv(envvars.CliTenantAccessToken, "")
			t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
			t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")

			multi := &configpkg.MultiAppConfig{
				CurrentApp: "default",
				Apps: []configpkg.AppConfig{
					{Name: "default", AppId: "app-default", AppSecret: secret.PlainSecret("secret-default"), Brand: brand.Feishu},
					{Name: "session", AppId: "app-session", AppSecret: secret.PlainSecret("secret-session"), Brand: brand.Feishu},
				},
			}
			if err := configpkg.SaveMultiAppConfig(multi); err != nil {
				t.Fatalf("SaveMultiAppConfig() error = %v", err)
			}

			code, stdout, stderr := executeWithCapturedOS(t, nil, tc.args...)
			if code != 0 {
				t.Fatalf("ExecuteWithOptions() exit=%d stderr=%s", code, stderr)
			}
			var result struct {
				Profile       string `json:"profile"`
				ProfileSource string `json:"profileSource"`
				AppID         string `json:"appId"`
			}
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("decode stdout: %v\nstdout: %s", err, stdout)
			}
			if result.Profile != tc.wantProfile || result.AppID != "app-"+tc.wantProfile {
				t.Fatalf("config show = %+v, want profile %q", result, tc.wantProfile)
			}
			if result.ProfileSource != tc.wantSource {
				t.Fatalf("profileSource = %q, want %q", result.ProfileSource, tc.wantSource)
			}
			saved, err := configpkg.LoadMultiAppConfig()
			if err != nil {
				t.Fatalf("LoadMultiAppConfig() error = %v", err)
			}
			if saved.CurrentApp != "default" {
				t.Fatalf("ephemeral selector persisted currentApp = %q", saved.CurrentApp)
			}
		})
	}
}

func TestExecuteEnvironmentProfileNotFoundNamesTheVariable(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	t.Setenv(envvars.CliProfile, "ghost")
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")

	multi := &configpkg.MultiAppConfig{
		CurrentApp: "default",
		Apps: []configpkg.AppConfig{
			{Name: "default", AppId: "app-default", AppSecret: secret.PlainSecret("secret-default"), Brand: brand.Feishu},
		},
	}
	if err := configpkg.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	// config default-as previously answered this input error with the generic
	// "no active profile" + a `config init --new` hint — active misdirection
	// for an agent: init is a destructive OAuth setup flow while an intact
	// profile sits in config.json. The envelope must name the environment
	// variable and steer toward unsetting or fixing it.
	for _, target := range [][]string{
		{"config", "default-as"},
		{"config", "show"},
		{"auth", "list", "--json"},
	} {
		t.Run(target[len(target)-1], func(t *testing.T) {
			code, stdout, stderr := executeWithCapturedOS(t, nil, target...)
			if code != output.ExitAuth {
				t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, output.ExitAuth, stdout, stderr)
			}
			var envelope struct {
				OK    bool `json:"ok"`
				Error struct {
					Type    errs.Category `json:"type"`
					Subtype errs.Subtype  `json:"subtype"`
					Message string        `json:"message"`
					Hint    string        `json:"hint"`
					Field   string        `json:"field"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
				t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, stderr)
			}
			if envelope.OK ||
				envelope.Error.Type != errs.CategoryConfig ||
				envelope.Error.Subtype != errs.SubtypeNotConfigured ||
				envelope.Error.Field != envvars.CliProfile {
				t.Errorf("envelope = %+v, want ok=false config/not_configured field=%s", envelope, envvars.CliProfile)
			}
			if want := `profile "ghost" not found`; envelope.Error.Message != want {
				t.Errorf("message = %q, want %q", envelope.Error.Message, want)
			}
			if !strings.Contains(envelope.Error.Hint, "unset "+envvars.CliProfile) {
				t.Errorf("hint = %q, want unset guidance", envelope.Error.Hint)
			}
			if strings.Contains(envelope.Error.Hint, "config init") {
				t.Errorf("hint must not steer to config init: %q", envelope.Error.Hint)
			}
			// auth list must not disguise the input error as an empty account.
			if strings.Contains(stdout, "not_logged_in") {
				t.Errorf("stdout still reports not_logged_in:\n%s", stdout)
			}
		})
	}
}
