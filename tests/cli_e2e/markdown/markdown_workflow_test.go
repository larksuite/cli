// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMarkdownLifecycleWorkflow(t *testing.T) {
	if os.Getenv("LARK_MARKDOWN_E2E") == "" {
		t.Skip("set LARK_MARKDOWN_E2E=1 to run markdown live workflow after backend version support is deployed")
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clie2e.SkipWithoutUserToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	fileName := "lark-cli-e2e-markdown-" + suffix + ".md"
	initialContent := "# Initial\n\nhello markdown workflow\n"
	patchedContent := "# Initial\n\nhello patched workflow\n"
	updatedContent := "# Updated\n\nnew body\n"

	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+create",
			"--name", fileName,
			"--content", initialContent,
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)

	fileToken := gjson.Get(createResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, fileToken, "stdout:\n%s", createResult.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"drive", "+delete",
				"--file-token", fileToken,
				"--type", "file",
				"--yes",
			},
			DefaultAs: "user",
		})
		clie2e.ReportCleanupFailure(parentT, "delete markdown file "+fileToken, deleteResult, deleteErr)
	})

	fetchInitialResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+fetch",
			"--file-token", fileToken,
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	fetchInitialResult.AssertExitCode(t, 0)
	fetchInitialResult.AssertStdoutStatus(t, true)
	require.Equal(t, initialContent, gjson.Get(fetchInitialResult.Stdout, "data.content").String(), "stdout:\n%s", fetchInitialResult.Stdout)

	patchResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+patch",
			"--file-token", fileToken,
			"--pattern", "hello markdown workflow",
			"--content", "hello patched workflow",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	patchResult.AssertExitCode(t, 0)
	patchResult.AssertStdoutStatus(t, true)
	require.Equal(t, true, gjson.Get(patchResult.Stdout, "data.updated").Bool(), "stdout:\n%s", patchResult.Stdout)
	require.Equal(t, int64(1), gjson.Get(patchResult.Stdout, "data.match_count").Int(), "stdout:\n%s", patchResult.Stdout)
	require.NotEmpty(t, gjson.Get(patchResult.Stdout, "data.version").String(), "stdout:\n%s", patchResult.Stdout)

	fetchPatchedResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+fetch",
			"--file-token", fileToken,
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	fetchPatchedResult.AssertExitCode(t, 0)
	fetchPatchedResult.AssertStdoutStatus(t, true)
	require.Equal(t, patchedContent, gjson.Get(fetchPatchedResult.Stdout, "data.content").String(), "stdout:\n%s", fetchPatchedResult.Stdout)

	overwriteResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+overwrite",
			"--file-token", fileToken,
			"--content", updatedContent,
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	overwriteResult.AssertExitCode(t, 0)
	overwriteResult.AssertStdoutStatus(t, true)
	require.NotEmpty(t, gjson.Get(overwriteResult.Stdout, "data.version").String(), "stdout:\n%s", overwriteResult.Stdout)

	fetchUpdatedResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+fetch",
			"--file-token", fileToken,
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	fetchUpdatedResult.AssertExitCode(t, 0)
	fetchUpdatedResult.AssertStdoutStatus(t, true)
	require.Equal(t, updatedContent, gjson.Get(fetchUpdatedResult.Stdout, "data.content").String(), "stdout:\n%s", fetchUpdatedResult.Stdout)
}
