// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestWiki_NodeCreateFileShortcutWorkflow verifies that +node-create can create
// a shortcut to a file-backed Wiki node. Every resource is created during the
// test and registered with the existing cleanup helpers.
func TestWiki_NodeCreateFileShortcutWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderToken := createDriveFolderOrSkipWikiSource(t, parentT, ctx, "lark-cli-e2e-wiki-file-shortcut-"+suffix)
	fileToken := uploadWikiSourceFixture(t, parentT, ctx, folderToken, "file-shortcut.txt", "wiki file shortcut "+suffix+"\n")

	_, parentNode := createWikiNodeUnderAnyHost(t, parentT, ctx, "lark-cli-e2e-wiki-file-shortcut-parent-"+suffix)
	spaceID := parentNode.Get("space_id").String()
	parentNodeToken := parentNode.Get("node_token").String()
	require.NotEmpty(t, spaceID)
	require.NotEmpty(t, parentNodeToken)

	fileNodeToken := moveDriveFileIntoWikiOrSkip(t, ctx, spaceID, parentNodeToken, fileToken)
	require.NotEmpty(t, fileNodeToken)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"wiki", "+node-create",
			"--space-id", spaceID,
			"--parent-node-token", parentNodeToken,
			"--node-type", "shortcut",
			"--obj-type", "file",
			"--origin-node-token", fileNodeToken,
			"--title", "lark-cli-e2e-wiki-file-shortcut-" + suffix,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	shortcutNodeToken := gjson.Get(result.Stdout, "data.node_token").String()
	require.NotEmpty(t, shortcutNodeToken, "stdout:\n%s", result.Stdout)
	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		deleteResult, deleteErr := deleteWikiNodeAndVerify(cleanupCtx, spaceID, shortcutNodeToken, "file")
		clie2e.ReportCleanupFailure(parentT, "delete wiki file shortcut "+shortcutNodeToken, deleteResult, deleteErr)
	})

	shortcut := getWikiNode(t, ctx, shortcutNodeToken)
	require.Equal(t, "shortcut", shortcut.Get("node_type").String(), "node:\n%s", shortcut.Raw)
	require.Equal(t, "file", shortcut.Get("obj_type").String(), "node:\n%s", shortcut.Raw)
	require.Equal(t, fileNodeToken, shortcut.Get("origin_node_token").String(), "node:\n%s", shortcut.Raw)
}
