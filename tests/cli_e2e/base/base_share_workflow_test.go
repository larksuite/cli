// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseShareWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_BASE_SHARE_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_BASE_SHARE_READY=1 after the dashboard/form share OpenAPI is deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-base-share-"+clie2e.GenerateSuffix())
	tableID, _, _ := createTableWithRetry(
		t,
		parentT,
		ctx,
		baseToken,
		"Share workflow "+clie2e.GenerateSuffix(),
		`[{"name":"Name","type":"text"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	formCreate, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-create",
			"--base-token", baseToken,
			"--table-id", tableID,
			"--name", "Share form " + clie2e.GenerateSuffix(),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	formCreate.AssertExitCode(t, 0)
	formCreate.AssertStdoutStatus(t, true)
	formID := gjson.Get(formCreate.Stdout, "data.id").String()
	require.NotEmpty(t, formID, formCreate.Stdout)

	dashboardCreate, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-create",
			"--base-token", baseToken,
			"--name", "Share dashboard " + clie2e.GenerateSuffix(),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	dashboardCreate.AssertExitCode(t, 0)
	dashboardCreate.AssertStdoutStatus(t, true)
	dashboardID := gjson.Get(dashboardCreate.Stdout, "data.dashboard.dashboard_id").String()
	require.NotEmpty(t, dashboardID, dashboardCreate.Stdout)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		for _, args := range [][]string{
			{"base", "+dashboard-share-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--enabled=false"},
			{"base", "+form-share-update", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID, "--enabled=false"},
		} {
			result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{Args: args, DefaultAs: "bot"})
			if cleanupErr != nil || result == nil || result.ExitCode != 0 {
				reportCleanupFailure(parentT, "disable share", result, cleanupErr)
			}
		}
	})

	t.Run("dashboard share update and get", func(t *testing.T) {
		runUpdate := func(fieldArgs ...string) {
			args := append([]string{
				"base", "+dashboard-share-update",
				"--base-token", baseToken,
				"--dashboard-id", dashboardID,
			}, fieldArgs...)
			update, runErr := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, runErr)
			update.AssertExitCode(t, 0)
			update.AssertStdoutStatus(t, true)
		}
		runUpdate("--enabled=true")
		runUpdate("--access-scope", "invite")
		runUpdate("--show-source=true")
		runUpdate("--show-source=false")

		get, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+dashboard-share-get", "--base-token", baseToken, "--dashboard-id", dashboardID},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		get.AssertExitCode(t, 0)
		get.AssertStdoutStatus(t, true)
		require.True(t, gjson.Get(get.Stdout, "data.enabled").Bool(), get.Stdout)
		require.Equal(t, "invite", gjson.Get(get.Stdout, "data.access_scope").String(), get.Stdout)
		showSource := gjson.Get(get.Stdout, "data.settings.show_source")
		require.True(t, showSource.Exists(), get.Stdout)
		require.False(t, showSource.Bool(), get.Stdout)
	})

	t.Run("form share update and get", func(t *testing.T) {
		runUpdate := func(fieldArgs ...string) {
			args := append([]string{
				"base", "+form-share-update",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--form-id", formID,
			}, fieldArgs...)
			update, runErr := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, runErr)
			update.AssertExitCode(t, 0)
			update.AssertStdoutStatus(t, true)
		}
		runUpdate("--enabled=true")
		runUpdate("--access-scope", "invite")
		runUpdate("--allow-anonymous=true")
		runUpdate("--require-login=true")
		runUpdate("--allow-anonymous=false")

		get, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+form-share-get", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		get.AssertExitCode(t, 0)
		get.AssertStdoutStatus(t, true)
		require.True(t, gjson.Get(get.Stdout, "data.enabled").Bool(), get.Stdout)
		require.Equal(t, "invite", gjson.Get(get.Stdout, "data.access_scope").String(), get.Stdout)
		allowAnonymous := gjson.Get(get.Stdout, "data.settings.allow_anonymous")
		require.True(t, allowAnonymous.Exists(), get.Stdout)
		require.False(t, allowAnonymous.Bool(), get.Stdout)
		requireLogin := gjson.Get(get.Stdout, "data.settings.require_login")
		require.True(t, requireLogin.Exists(), get.Stdout)
		require.True(t, requireLogin.Bool(), get.Stdout)
	})
}
