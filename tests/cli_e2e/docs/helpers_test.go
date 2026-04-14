// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func createDocsFolderWithRetry(t *testing.T, ctx context.Context, name string) string {
	t.Helper()

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{"drive", "files", "create_folder"},
		Data: map[string]any{
			"name":         name,
			"folder_token": "",
		},
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	folderToken := gjson.Get(result.Stdout, "data.token").String()
	require.NotEmpty(t, folderToken, "stdout:\n%s", result.Stdout)

	return folderToken
}

func createDocWithRetry(t *testing.T, ctx context.Context, folderToken string, title string, markdown string) string {
	t.Helper()

	require.NotEmpty(t, folderToken, "folder token is required")

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--folder-token", folderToken,
			"--title", title,
			"--markdown", markdown,
		},
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	docToken := gjson.Get(result.Stdout, "data.doc_id").String()
	require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)
	return docToken
}
