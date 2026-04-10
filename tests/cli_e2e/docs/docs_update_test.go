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
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	originalTitle := "lark-cli-e2e-update-" + suffix
	updatedTitle := "lark-cli-e2e-update-updated-" + suffix
	originalContent := "# Original\n\nThis is the original content."
	updatedContent := "# Updated\n\nThis is the updated content."

	var docToken string

	t.Run("create", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+create",
				"--title", originalTitle,
				"--markdown", originalContent,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		docToken = gjson.Get(result.Stdout, "data.doc_id").String()
		require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			// best-effort cleanup
		})
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