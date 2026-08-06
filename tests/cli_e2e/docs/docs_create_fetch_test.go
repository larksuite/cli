// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-docs-folder-" + suffix
	docTitle := "lark-cli-e2e-docs-" + suffix
	tailMarker := "docs-full-spill-tail-" + suffix
	docContent := "# Test Document\n\nThis document was created by lark-cli e2e test.\n\n" +
		strings.Repeat("Large document content for automatic spill verification.\n\n", 600) + tailMarker

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
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		content := gjson.Get(result.Stdout, "data.document.content").String()
		assert.Contains(t, content, docTitle)
		assert.Contains(t, content, "This document was created by lark-cli e2e test.")
	})

	t.Run("fetch full with automatic spill", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before fetch")
		t.Setenv("TMPDIR", t.TempDir())

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", docToken,
				"--doc-format", "markdown",
				"--full",
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		document := gjson.Get(result.Stdout, "data.document")
		require.True(t, document.Exists(), "stdout:\n%s", result.Stdout)
		assert.False(t, document.Get("content").Exists(), "stdout:\n%s", result.Stdout)
		contentInline := document.Get("content_inline")
		require.True(t, contentInline.Exists(), "stdout:\n%s", result.Stdout)
		assert.False(t, contentInline.Bool(), "stdout:\n%s", result.Stdout)
		temporary := document.Get("content_file.temporary")
		require.True(t, temporary.Exists(), "stdout:\n%s", result.Stdout)
		assert.True(t, temporary.Bool(), "stdout:\n%s", result.Stdout)
		assert.NotEmpty(t, document.Get("content_preview").String(), "stdout:\n%s", result.Stdout)

		outputPath := document.Get("content_file.path").String()
		require.True(t, filepath.IsAbs(outputPath), "content_file.path=%q", outputPath)
		t.Cleanup(func() { require.NoError(t, os.Remove(outputPath), "remove spill file %q", outputPath) })
		saved, readErr := os.ReadFile(outputPath)
		require.NoError(t, readErr)
		assert.Greater(t, len(saved), 24*1024)
		assert.Contains(t, string(saved), tailMarker)
		assert.Equal(t, int64(len(saved)), document.Get("content_file.size_bytes").Int())
		assert.Equal(t, "utf-8", document.Get("content_file.encoding").String())
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
