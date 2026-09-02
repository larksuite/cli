// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestBaseAppWorkflow exercises the live BaseApp lifecycle inside a dedicated
// fixture Workspace. Workspace has no delete shortcut, so the fixture token is
// supplied by the environment while every App/Page/Block created here is
// cleaned up by the test.
func TestBaseAppWorkflow(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	workspaceToken := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_BASEAPP_WORKSPACE_TOKEN"))
	if workspaceToken == "" {
		t.Skip("set LARK_CLI_E2E_BASEAPP_WORKSPACE_TOKEN to a dedicated Workspace for the live BaseApp workflow")
	}

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	appToken := ""
	pageID := ""
	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		if pageID != "" && appToken != "" {
			result, err := runBaseAppLive(cleanupCtx, "base", "+app-page-delete", "--app-token", appToken, "--page-id", pageID, "--yes")
			if err != nil || result.ExitCode != 0 {
				reportCleanupFailure(parentT, "delete BaseApp page "+pageID, result, err)
			}
		}
		if appToken != "" {
			result, err := runBaseAppLive(cleanupCtx, "drive", "+delete", "--file-token", appToken, "--type", "bitable", "--yes")
			if err != nil || result.ExitCode != 0 {
				reportCleanupFailure(parentT, "delete BaseApp "+appToken, result, err)
			}
		}
	})

	workspaceList, err := runBaseAppLive(ctx, "base", "+workspace-entity-list", "--workspace-token", workspaceToken, "--page-size", "100")
	require.NoError(t, err)
	workspaceList.AssertExitCode(t, 0)
	workspaceList.AssertStdoutStatus(t, true)

	appName := "lark-cli-e2e-app-" + clie2e.GenerateSuffix()
	createApp, err := runBaseAppLive(ctx, "base", "+app-create", "--workspace-token", workspaceToken, "--name", appName)
	require.NoError(t, err)
	createApp.AssertExitCode(t, 0)
	createApp.AssertStdoutStatus(t, true)
	appToken = firstLiveValue(createApp.Stdout, "data.app.app_token", "data.app.app_id", "data.app.token")
	require.NotEmpty(t, appToken, "stdout:\n%s", createApp.Stdout)

	getApp, err := runBaseAppLive(ctx, "base", "+app-get", "--app-token", appToken)
	require.NoError(t, err)
	getApp.AssertExitCode(t, 0)
	getApp.AssertStdoutStatus(t, true)
	require.Equal(t, appToken, firstLiveValue(getApp.Stdout, "data.app_token", "data.app_id", "data.token"), "stdout:\n%s", getApp.Stdout)

	pageName := "Overview-" + clie2e.GenerateSuffix()
	createPage, err := runBaseAppLive(ctx, "base", "+app-page-create", "--app-token", appToken, "--name", pageName)
	require.NoError(t, err)
	createPage.AssertExitCode(t, 0)
	createPage.AssertStdoutStatus(t, true)
	pageID = firstLiveValue(createPage.Stdout, "data.page.page_id", "data.page.id")
	require.NotEmpty(t, pageID, "stdout:\n%s", createPage.Stdout)

	pageList, err := runBaseAppLive(ctx, "base", "+app-page-list", "--app-token", appToken, "--page-size", "100")
	require.NoError(t, err)
	pageList.AssertExitCode(t, 0)
	pageList.AssertStdoutStatus(t, true)
	require.Contains(t, pageList.Stdout, pageID)

	updatedPageName := pageName + " Updated"
	updatePage, err := runBaseAppLive(ctx, "base", "+app-page-update", "--app-token", appToken, "--page-id", pageID, "--name", updatedPageName)
	require.NoError(t, err)
	updatePage.AssertExitCode(t, 0)
	updatePage.AssertStdoutStatus(t, true)

	getPage, err := runBaseAppLive(ctx, "base", "+app-page-get", "--app-token", appToken, "--page-id", pageID)
	require.NoError(t, err)
	getPage.AssertExitCode(t, 0)
	getPage.AssertStdoutStatus(t, true)
	require.Equal(t, updatedPageName, firstLiveValue(getPage.Stdout, "data.page.name"), "stdout:\n%s", getPage.Stdout)

	blockName := "Notes-" + clie2e.GenerateSuffix()
	createBlock, err := runBaseAppLive(ctx, "base", "+app-block-create", "--app-token", appToken, "--page-id", pageID, "--name", blockName, "--type", "text", "--data-config", `{"text":"# Initial"}`)
	require.NoError(t, err)
	createBlock.AssertExitCode(t, 0)
	createBlock.AssertStdoutStatus(t, true)
	blockID := firstLiveValue(createBlock.Stdout, "data.block.block_id", "data.block.id", "data.block.widget_id")
	require.NotEmpty(t, blockID, "stdout:\n%s", createBlock.Stdout)

	blockList, err := runBaseAppLive(ctx, "base", "+app-block-list", "--app-token", appToken, "--page-id", pageID, "--page-size", "100")
	require.NoError(t, err)
	blockList.AssertExitCode(t, 0)
	blockList.AssertStdoutStatus(t, true)
	require.Contains(t, blockList.Stdout, blockID)

	updatedBlockName := blockName + " Updated"
	updateBlock, err := runBaseAppLive(ctx, "base", "+app-block-update", "--app-token", appToken, "--page-id", pageID, "--block-id", blockID, "--name", updatedBlockName, "--data-config", `{"text":"# Updated"}`)
	require.NoError(t, err)
	updateBlock.AssertExitCode(t, 0)
	updateBlock.AssertStdoutStatus(t, true)

	getBlock, err := runBaseAppLive(ctx, "base", "+app-block-get", "--app-token", appToken, "--page-id", pageID, "--block-id", blockID)
	require.NoError(t, err)
	getBlock.AssertExitCode(t, 0)
	getBlock.AssertStdoutStatus(t, true)
	require.Equal(t, updatedBlockName, firstLiveValue(getBlock.Stdout, "data.block.name"), "stdout:\n%s", getBlock.Stdout)
	require.Equal(t, "# Updated", firstLiveValue(getBlock.Stdout, "data.block.data_config.text"), "stdout:\n%s", getBlock.Stdout)

	deletePage, err := runBaseAppLive(ctx, "base", "+app-page-delete", "--app-token", appToken, "--page-id", pageID, "--yes")
	require.NoError(t, err)
	deletePage.AssertExitCode(t, 0)
	deletePage.AssertStdoutStatus(t, true)
	pageID = ""
}

func runBaseAppLive(ctx context.Context, args ...string) (*clie2e.Result, error) {
	return clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
}

func firstLiveValue(stdout string, paths ...string) string {
	for _, path := range paths {
		if value := strings.TrimSpace(gjson.Get(stdout, path).String()); value != "" {
			return value
		}
	}
	return ""
}
