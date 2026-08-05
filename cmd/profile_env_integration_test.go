// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

func TestExecuteProfileSelectorPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name        string
		envProfile  string
		args        []string
		wantProfile string
	}{
		{"environment selects profile", "session", []string{"config", "show"}, "session"},
		{"flag overrides environment", "session", []string{"config", "show", "--profile", "default"}, "default"},
		{"empty flag uses persisted default", "session", []string{"config", "show", "--profile="}, "default"},
		{"empty environment uses persisted default", "", []string{"config", "show"}, "default"},
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

			multi := &core.MultiAppConfig{
				CurrentApp: "default",
				Apps: []core.AppConfig{
					{Name: "default", AppId: "app-default", AppSecret: core.PlainSecret("secret-default"), Brand: core.BrandFeishu},
					{Name: "session", AppId: "app-session", AppSecret: core.PlainSecret("secret-session"), Brand: core.BrandFeishu},
				},
			}
			if err := core.SaveMultiAppConfig(multi); err != nil {
				t.Fatalf("SaveMultiAppConfig() error = %v", err)
			}

			code, stdout, stderr := executeWithCapturedOS(t, nil, tc.args...)
			if code != 0 {
				t.Fatalf("ExecuteWithOptions() exit=%d stderr=%s", code, stderr)
			}
			var result struct {
				Profile string `json:"profile"`
				AppID   string `json:"appId"`
			}
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("decode stdout: %v\nstdout: %s", err, stdout)
			}
			if result.Profile != tc.wantProfile || result.AppID != "app-"+tc.wantProfile {
				t.Fatalf("config show = %+v, want profile %q", result, tc.wantProfile)
			}
			saved, err := core.LoadMultiAppConfig()
			if err != nil {
				t.Fatalf("LoadMultiAppConfig() error = %v", err)
			}
			if saved.CurrentApp != "default" {
				t.Fatalf("ephemeral selector persisted currentApp = %q", saved.CurrentApp)
			}
		})
	}
}
