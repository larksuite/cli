// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_UpdateWorkflow tests the create, update, and verify lifecycle.
func TestDocs_UpdateWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-update-folder-" + suffix
	originalTitle := "lark-cli-e2e-update-" + suffix
	updatedTitle := "lark-cli-e2e-update-updated-" + suffix
	originalContent := "# Original\n\nThis is the original content."
	updatedContent := "# Updated\n\nThis is the updated content."

	folderToken := createDocsFolderWithRetry(t, ctx, folderName)
	var docToken string

	t.Run("create", func(t *testing.T) {
		docToken = createDocWithRetry(t, ctx, folderToken, originalTitle, originalContent)
	})

	t.Run("update-title-and-content", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before update")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", docToken,
				"--mode", "overwrite",
				"--markdown", updatedContent,
				"--new-title", updatedTitle,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("verify", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before verify")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", docToken,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, updatedTitle, gjson.Get(result.Stdout, "data.title").String())
	})
}
