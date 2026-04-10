// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_FilesCreateFolderWorkflow tests the files create_folder resource command.
func TestDrive_FilesCreateFolderWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	folderName := "lark-cli-e2e-drive-folder-" + suffix

	var folderToken string

	t.Run("create_folder", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "files", "create_folder"},
			Data: map[string]any{
				"name":         folderName,
				"folder_token": "",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		folderToken = gjson.Get(result.Stdout, "data.token").String()
		require.NotEmpty(t, folderToken, "folder token should be available, stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"drive", "files", "delete"},
				Params: map[string]any{"file_token": folderToken, "type": "folder"},
			})
		})
	})
}
