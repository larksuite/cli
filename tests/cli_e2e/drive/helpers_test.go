// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// createDriveFolder creates a private folder for the current workflow and
// deletes it during cleanup.
func createDriveFolder(t *testing.T, parentT *testing.T, ctx context.Context, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "files", "create_folder"},
		DefaultAs: "bot",
		Data: map[string]any{
			"name":         name,
			"folder_token": "",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	folderToken := gjson.Get(result.Stdout, "data.token").String()
	require.NotEmpty(t, folderToken, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:      []string{"drive", "files", "delete"},
			DefaultAs: "bot",
			Params:    map[string]any{"file_token": folderToken, "type": "folder"},
		})
	})

	return folderToken
}
