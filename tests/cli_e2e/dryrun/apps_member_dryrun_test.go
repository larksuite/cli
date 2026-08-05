// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dryrun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/cmd"
	_ "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/internal/registry/registrytest"
	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	appsMemberHelperEnv = "LARK_CLI_APPS_MEMBER_HELPER"
	appsMemberRootEnv   = "LARK_CLI_APPS_MEMBER_TEST_ROOT"
)

// TestAppsMemberCLIHelperProcess re-executes the current test binary as the
// real CLI entry point. This keeps the E2E proof on the code under test without
// requiring a repository-wide standalone `go build`.
func TestAppsMemberCLIHelperProcess(t *testing.T) {
	if os.Getenv(appsMemberHelperEnv) != "1" {
		return
	}
	separator := -1
	for index, arg := range os.Args {
		if arg == "--" {
			separator = index
			break
		}
	}
	if separator < 0 {
		os.Exit(2)
	}
	if err := registrytest.Seed(os.Getenv(appsMemberRootEnv)); err != nil {
		fmt.Fprintln(os.Stderr, "seed API metadata fixture:", err)
		os.Exit(2)
	}
	os.Args = append([]string{"lark-cli"}, os.Args[separator+1:]...)
	os.Exit(cmd.Execute())
}

func runAppsMemberCLI(t *testing.T, args ...string) *clie2e.Result {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	testRoot := t.TempDir()
	requestArgs := append([]string{"-test.run=^TestAppsMemberCLIHelperProcess$", "--"}, args...)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		BinaryPath: executable,
		Args:       requestArgs,
		DefaultAs:  "user",
		Env: map[string]string{
			appsMemberHelperEnv:                "1",
			appsMemberRootEnv:                  testRoot,
			"LARKSUITE_CLI_CONFIG_DIR":         filepath.Join(testRoot, "config"),
			"LARKSUITE_CLI_APP_ID":             "apps_member_dryrun_client",
			"LARKSUITE_CLI_APP_SECRET":         "apps_member_dryrun_secret",
			"LARKSUITE_CLI_USER_ACCESS_TOKEN":  "apps_member_dryrun_user_token",
			"LARKSUITE_CLI_BRAND":              "feishu",
			"LARKSUITE_CLI_NO_UPDATE_NOTIFIER": "1",
			"LARKSUITE_CLI_NO_SKILLS_NOTIFIER": "1",
		},
	})
	require.NoError(t, err)
	return result
}

func TestAppsMemberDryRun(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		method string
		url    string
		assert func(*testing.T, string)
	}{
		{
			name: "list", args: []string{"apps", "+member-list", "--app-id", " app_报告 ", "--member-type", "chat", "--dry-run"},
			method: "GET", url: "/open-apis/spark/v1/apps/app_%E6%8A%A5%E5%91%8A/members",
			assert: func(t *testing.T, out string) {
				require.Equal(t, "chat", clie2e.DryRunGet(out, "api.0.params.member_type").String())
				require.False(t, clie2e.DryRunGet(out, "api.0.params.page_size").Exists())
				require.False(t, clie2e.DryRunGet(out, "api.0.params.page_token").Exists())
			},
		},
		{
			name: "add", args: []string{"apps", "+member-add", "--app-id", "app_x", "--member-type", "openid", "--member-id", "ou_user", "--perm", "view", "--need-notification=false", "--dry-run"},
			method: "POST", url: "/open-apis/spark/v1/apps/app_x/members",
			assert: func(t *testing.T, out string) {
				require.Equal(t, "ou_user", clie2e.DryRunGet(out, "api.0.body.user_open_id").String())
				require.False(t, clie2e.DryRunGet(out, "api.0.body.need_notification").Bool())
				require.False(t, clie2e.DryRunGet(out, "api.0.body.department_id").Exists())
				require.False(t, clie2e.DryRunGet(out, "api.0.body.chat_id").Exists())
			},
		},
		{
			name: "update", args: []string{"apps", "+member-update", "--app-id", "app_x", "--member-type", "openchat", "--member-id", "oc_chat", "--perm", "edit", "--dry-run"},
			method: "PATCH", url: "/open-apis/spark/v1/apps/app_x/members",
			assert: func(t *testing.T, out string) {
				require.Equal(t, "oc_chat", clie2e.DryRunGet(out, "api.0.body.chat_id").String())
				require.Equal(t, "edit", clie2e.DryRunGet(out, "api.0.body.role").String())
				require.False(t, clie2e.DryRunGet(out, "api.0.body.user_open_id").Exists())
			},
		},
		{
			name: "remove", args: []string{"apps", "+member-remove", "--app-id", "app_x", "--member-type", "opendepartmentid", "--member-id", "od-department", "--dry-run"},
			method: "POST", url: "/open-apis/spark/v1/apps/app_x/members/remove",
			assert: func(t *testing.T, out string) {
				require.Equal(t, "od-department", clie2e.DryRunGet(out, "api.0.body.department_id").String())
				require.False(t, clie2e.DryRunGet(out, "api.0.body.user_open_id").Exists())
				require.False(t, clie2e.DryRunGet(out, "api.0.body.chat_id").Exists())
			},
		},
		{
			name: "settings-get", args: []string{"apps", "+member-settings-get", "--app-id", "app_x", "--dry-run"},
			method: "GET", url: "/open-apis/spark/v1/apps/app_x/member-settings",
			assert: func(t *testing.T, out string) {
				require.False(t, clie2e.DryRunGet(out, "api.0.body").Exists())
			},
		},
		{
			name: "settings-set", args: []string{"apps", "+member-settings-set", "--app-id", "app_x", "--external-access", "disabled", "--copy-download-by", "full-access", "--dry-run"},
			method: "PATCH", url: "/open-apis/spark/v1/apps/app_x/member-settings",
			assert: func(t *testing.T, out string) {
				require.Equal(t, "disabled", clie2e.DryRunGet(out, "api.0.body.external_access").String())
				require.Equal(t, "full-access", clie2e.DryRunGet(out, "api.0.body.copy_download_by").String())
				require.False(t, clie2e.DryRunGet(out, "api.0.body.link_share").Exists())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runAppsMemberCLI(t, tc.args...)
			result.AssertExitCode(t, 0)
			require.Equal(t, tc.method, clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			require.Equal(t, tc.url, clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			tc.assert(t, result.Stdout)
		})
	}
}

func TestAppsMemberHighRiskWritesRequireYesButDryRunDoesNot(t *testing.T) {
	writes := [][]string{
		{"apps", "+member-add", "--app-id", "app_x", "--member-type", "openid", "--member-id", "ou_user", "--perm", "view"},
		{"apps", "+member-update", "--app-id", "app_x", "--member-type", "openchat", "--member-id", "oc_chat", "--perm", "edit"},
		{"apps", "+member-remove", "--app-id", "app_x", "--member-type", "opendepartmentid", "--member-id", "od-department"},
		{"apps", "+member-settings-set", "--app-id", "app_x", "--external-access", "enabled"},
	}
	for _, args := range writes {
		t.Run(args[1], func(t *testing.T) {
			result := runAppsMemberCLI(t, args...)
			result.AssertExitCode(t, 10)
			require.Equal(t, "confirmation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
			require.Equal(t, "confirmation_required", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "--yes")

			dryArgs := append(append([]string{}, args...), "--dry-run")
			dryResult := runAppsMemberCLI(t, dryArgs...)
			dryResult.AssertExitCode(t, 0)
			require.True(t, gjson.Get(dryResult.Stdout, "dry_run").Bool(), "stdout:\n%s\nstderr:\n%s", dryResult.Stdout, dryResult.Stderr)
		})
	}
}

func TestAppsMemberValidationFailuresAreStructured(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		param string
	}{
		{name: "missing-app-list", args: []string{"apps", "+member-list", "--dry-run"}, param: "--app-id"},
		{name: "missing-app-add", args: []string{"apps", "+member-add", "--member-type", "openid", "--member-id", "ou_user", "--perm", "view", "--dry-run"}, param: "--app-id"},
		{name: "missing-app-update", args: []string{"apps", "+member-update", "--member-type", "openid", "--member-id", "ou_user", "--perm", "edit", "--dry-run"}, param: "--app-id"},
		{name: "missing-app-remove", args: []string{"apps", "+member-remove", "--member-type", "openid", "--member-id", "ou_user", "--dry-run"}, param: "--app-id"},
		{name: "missing-app-settings-get", args: []string{"apps", "+member-settings-get", "--dry-run"}, param: "--app-id"},
		{name: "missing-app-settings-set", args: []string{"apps", "+member-settings-set", "--external-access", "enabled", "--dry-run"}, param: "--app-id"},
		{name: "mismatched-member-id", args: []string{"apps", "+member-add", "--app-id", "app_x", "--member-type", "openid", "--member-id", "123456789", "--perm", "view", "--dry-run"}, param: "--member-id"},
		{name: "setting-enum", args: []string{"apps", "+member-settings-set", "--app-id", "app_x", "--link-share", "internet-editable", "--dry-run"}, param: "--link-share"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runAppsMemberCLI(t, tc.args...)
			result.AssertExitCode(t, 2)
			require.Empty(t, result.Stdout, "validation failure must not write stdout")
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
			require.Equal(t, tc.param, gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)
			hint := gjson.Get(result.Stderr, "error.hint").String()
			message := gjson.Get(result.Stderr, "error.message").String()
			require.True(t, hint != "" || strings.Contains(message, "allowed:"), "validation error must provide recovery guidance, stderr:\n%s", result.Stderr)
		})
	}

	result := runAppsMemberCLI(t, "apps", "+member-settings-set", "--app-id", "app_x", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout, "validation failure must not write stdout")
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
	require.True(t, gjson.Get(result.Stderr, "error.params.#").Int() >= 1, "stderr:\n%s", result.Stderr)
}
