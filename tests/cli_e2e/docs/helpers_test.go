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

func createDocWithRetry(t *testing.T, parentT *testing.T, ctx context.Context, folderToken string, title string, markdown string, defaultAs string) string {
	t.Helper()

	require.NotEmpty(t, folderToken, "folder token is required")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--folder-token", folderToken,
			"--title", title,
			"--markdown", markdown,
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	docToken := gjson.Get(result.Stdout, "data.doc_id").String()
	require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"drive", "+delete",
				"--file-token", docToken,
				"--type", "docx",
				"--yes",
			},
			DefaultAs: defaultAs,
		})
		clie2e.ReportCleanupFailure(parentT, "delete doc "+docToken, deleteResult, deleteErr)
	})

	return docToken
}
