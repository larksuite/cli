// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestDocs_LocalResourcesDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	workDir := t.TempDir()
	writeLocalResourceFixture(t, workDir, "dry-run.png", hundredByEightyPNG)
	writeLocalResourceFixture(t, workDir, "dry-run.txt", []byte("dry-run source fixture\n"))
	content := `<p>dry-run resources</p><img path="@dry-run.png" caption="dry-run image" width="50"/><source path="@dry-run.txt" name="dry-run-report.txt"/>`

	tests := []struct {
		name            string
		args            []string
		wantDocumentURL string
		wantCommand     string
	}{
		{
			name: "create",
			args: []string{
				"docs", "+create",
				"--title", "Local resources dry-run",
				"--content", content,
				"--dry-run",
			},
			wantDocumentURL: "/open-apis/docs_ai/v1/documents",
		},
		{
			name: "update append",
			args: []string{
				"docs", "+update",
				"--doc", "doxcnLocalResourcesDryRun",
				"--command", "append",
				"--content", content,
				"--dry-run",
			},
			wantDocumentURL: "/open-apis/docs_ai/v1/documents/doxcnLocalResourcesDryRun",
			wantCommand:     "block_insert_after",
		},
		{
			name: "update overwrite",
			args: []string{
				"docs", "+update",
				"--doc", "doxcnLocalResourcesDryRun",
				"--command", "overwrite",
				"--content", content,
				"--dry-run",
			},
			wantDocumentURL: "/open-apis/docs_ai/v1/documents/doxcnLocalResourcesDryRun",
			wantCommand:     "overwrite",
		},
		{
			name: "update block replace",
			args: []string{
				"docs", "+update",
				"--doc", "doxcnLocalResourcesDryRun",
				"--command", "block_replace",
				"--block-id", "blkLocalResourceTarget",
				"--content", content,
				"--dry-run",
			},
			wantDocumentURL: "/open-apis/docs_ai/v1/documents/doxcnLocalResourcesDryRun",
			wantCommand:     "block_replace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "bot",
				WorkDir:   workDir,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			apis := clie2e.DryRunGet(result.Stdout, "api").Array()
			require.Len(t, apis, 6, "stdout:\n%s", result.Stdout)
			require.Equal(t, tt.wantDocumentURL, apis[0].Get("url").String(), "stdout:\n%s", result.Stdout)
			if tt.wantCommand != "" {
				require.Equal(t, tt.wantCommand, apis[0].Get("body.command").String(), "stdout:\n%s", result.Stdout)
			}
			if tt.wantCommand == "block_replace" {
				require.Equal(t, "blkLocalResourceTarget", apis[0].Get("body.block_id").String(), "stdout:\n%s", result.Stdout)
			}

			preparedContent := apis[0].Get("body.content").String()
			require.Contains(t, preparedContent, "dry-run image")
			require.Contains(t, preparedContent, "dry-run-report.txt")
			require.NotContains(t, preparedContent, "@dry-run.png")
			require.NotContains(t, preparedContent, "@dry-run.txt")
			require.Equal(t, 2, strings.Count(preparedContent, "@lcli_"), "prepared content:\n%s", preparedContent)

			require.Equal(t, "/open-apis/drive/v1/medias/upload_all", apis[1].Get("url").String())
			require.Equal(t, "docx_image", apis[1].Get("body.parent_type").String())
			require.Equal(t, "<local_image_1_block_id>", apis[1].Get("body.parent_node").String())
			require.Equal(t, "/open-apis/drive/v1/medias/upload_all", apis[2].Get("url").String())
			require.Equal(t, "docx_file", apis[2].Get("body.parent_type").String())
			require.Equal(t, "<local_file_2_block_id>", apis[2].Get("body.parent_node").String())

			require.Contains(t, apis[3].Get("url").String(), "/open-apis/docx/v1/documents/")
			require.Contains(t, apis[3].Get("url").String(), "/blocks/batch_update")
			require.NotEmpty(t, apis[3].Get("params.client_token").String())
			require.Equal(t, "<uploaded_file_token_1>", apis[3].Get("body.requests.0.replace_image.token").String())
			require.Equal(t, int64(100), apis[3].Get("body.requests.0.replace_image.width").Int())
			require.Equal(t, int64(80), apis[3].Get("body.requests.0.replace_image.height").Int())
			require.InDelta(t, 0.5, apis[3].Get("body.requests.0.replace_image.scale").Float(), 0.000001)
			require.Equal(t, "<uploaded_file_token_2>", apis[3].Get("body.requests.1.replace_file.token").String())

			require.Equal(t, "GET", apis[4].Get("method").String())
			require.Equal(t, "PUT", apis[5].Get("method").String())
			require.Contains(t, apis[5].Get("url").String(), "/open-apis/docs_ai/v1/documents/")
			require.Equal(t, "block_delete", apis[5].Get("body.command").String())
		})
	}
}

func TestDocs_RemoteImageDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--title", "Remote image dry-run",
			"--content", `<p>before</p><img href="https://93.184.216.34/photo.png?x=1&image_size=large" caption="remote image"/>`,
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   t.TempDir(),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	apis := clie2e.DryRunGet(result.Stdout, "api").Array()
	require.Len(t, apis, 6, "stdout:\n%s", result.Stdout)
	require.Equal(t, "POST", apis[0].Get("method").String())
	require.Equal(t, "/open-apis/docs_ai/v1/documents", apis[0].Get("url").String())
	preparedContent := apis[0].Get("body.content").String()
	require.NotContains(t, preparedContent, `href=`)
	require.Contains(t, preparedContent, "@lcli_img_")
	require.Contains(t, preparedContent, `caption="remote image"`)

	require.Equal(t, "GET", apis[1].Get("method").String())
	require.Equal(t, "https://93.184.216.34/photo.png", apis[1].Get("url").String())
	require.Contains(t, apis[1].Get("desc").String(), "query")
	require.Contains(t, apis[1].Get("desc").String(), "bounded concurrent upload worker")

	require.Equal(t, "/open-apis/drive/v1/medias/upload_all", apis[2].Get("url").String())
	require.Equal(t, "docx_image", apis[2].Get("body.parent_type").String())
	require.Equal(t, "<local_image_1_block_id>", apis[2].Get("body.parent_node").String())
}
