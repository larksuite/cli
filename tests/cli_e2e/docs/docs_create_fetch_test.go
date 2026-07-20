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

// TestDocs_CreateAndFetchWorkflow tests the create and fetch lifecycle.
func TestDocs_CreateAndFetchWorkflowAsBot(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-docs-folder-" + suffix
	docTitle := "lark-cli-e2e-docs-" + suffix
	docContent := "# Test Document\n\nThis document was created by lark-cli e2e test."

	const defaultAs = "bot"
	folderToken := drive.CreateDriveFolder(t, parentT, ctx, folderName, defaultAs, "")
	var docToken string

	t.Run("create", func(t *testing.T) {
		docToken = createDocWithRetry(t, parentT, ctx, folderToken, docTitle, docContent, defaultAs)
	})

	t.Run("fetch", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before fetch")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", docToken,
				"--doc-format", "markdown",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		content := gjson.Get(result.Stdout, "data.document.content").String()
		assert.Contains(t, content, docTitle)
		assert.Contains(t, content, "This document was created by lark-cli e2e test.")
	})
}

func TestDocs_CreateAndFetchWorkflowAsUser(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-user-docs-folder-" + suffix
	docTitle := "lark-cli-e2e-user-docs-" + suffix
	docContent := "# User Test Document\n\nCreated with user access token."
	var docToken string
	const defaultAs = "user"
	folderToken := drive.CreateDriveFolder(t, parentT, ctx, folderName, defaultAs, "")

	t.Run("create as user", func(t *testing.T) {
		docToken = createDocWithRetry(t, parentT, ctx, folderToken, docTitle, docContent, defaultAs)
	})

	t.Run("fetch as user", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before fetch")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "markdown"},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		content := gjson.Get(result.Stdout, "data.document.content").String()
		assert.Contains(t, content, docTitle)
		assert.Contains(t, content, "Created with user access token.")
	})
}
