// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDrive_FetchFullAutomaticSpillWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	t.Setenv("TMPDIR", t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	suffix := clie2e.GenerateSuffix()
	folderToken := createDriveFolder(t, t, ctx, "lark-cli-e2e-drive-fetch-"+suffix, "")
	tailMarker := "drive-full-spill-tail-" + suffix
	content := "# Fetch spill fixture\n\n" +
		strings.Repeat("Large document content for automatic spill verification.\n\n", 600) + tailMarker
	docToken := createDriveFetchWorkflowDoc(t, ctx, folderToken, content)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "+fetch", "--token", docToken, "--type", "docx", "--full"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	data := gjson.Get(result.Stdout, "data")
	require.True(t, data.Exists(), "stdout:\n%s", result.Stdout)
	require.False(t, data.Get("content").Exists(), "stdout:\n%s", result.Stdout)
	require.True(t, data.Get("content_inline").Exists(), "stdout:\n%s", result.Stdout)
	require.False(t, data.Get("content_inline").Bool(), "stdout:\n%s", result.Stdout)
	require.True(t, data.Get("content_file.temporary").Bool(), "stdout:\n%s", result.Stdout)
	require.NotEmpty(t, data.Get("content_preview").String(), "stdout:\n%s", result.Stdout)

	outputPath := data.Get("content_file.path").String()
	require.True(t, filepath.IsAbs(outputPath), "content_file.path=%q", outputPath)
	t.Cleanup(func() { require.NoError(t, os.Remove(outputPath), "remove spill file %q", outputPath) })
	saved, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	require.Greater(t, len(saved), 24*1024)
	require.Contains(t, string(saved), tailMarker)
	require.Equal(t, int64(len(saved)), data.Get("content_file.size_bytes").Int())
	require.Equal(t, "utf-8", data.Get("content_file.encoding").String())
}

func createDriveFetchWorkflowDoc(t *testing.T, ctx context.Context, folderToken, content string) string {
	t.Helper()
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--parent-token", folderToken,
			"--doc-format", "markdown",
			"--content", content,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	docToken := gjson.Get(result.Stdout, "data.document.document_id").String()
	require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)
	return docToken
}
