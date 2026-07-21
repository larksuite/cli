// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_UpdateWorkflow tests the create, update, and verify lifecycle.
func TestDocs_UpdateWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-update-folder-" + suffix
	originalTitle := "lark-cli-e2e-update-" + suffix
	updatedTitle := "lark-cli-e2e-update-updated-" + suffix
	originalContent := "# Original\n\nThis is the original content."
	updatedContent := "# Updated\n\nThis is the updated content."
	const defaultAs = "bot"

	folderToken := drive.CreateDriveFolder(t, parentT, ctx, folderName, defaultAs, "")
	var docToken string

	t.Run("create as bot", func(t *testing.T) {
		docToken = createDocWithRetry(t, parentT, ctx, folderToken, originalTitle, originalContent, defaultAs)
	})

	t.Run("update-title-and-content as bot", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before update")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", docToken,
				"--command", "overwrite",
				"--doc-format", "markdown",
				"--content", "# " + updatedTitle + "\n\n" + updatedContent,
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("verify as bot", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before verify")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", docToken,
				"--doc-format", "markdown",
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		content := gjson.Get(result.Stdout, "data.document.content").String()
		assert.Contains(t, content, updatedTitle)
		assert.Contains(t, content, "This is the updated content.")
	})
}
