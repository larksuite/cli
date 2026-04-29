// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMarkdownCreateDryRun_Content(t *testing.T) {
	setMarkdownDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+create",
			"--name", "README.md",
			"--content", "# hello",
			"--folder-token", "fldcnMarkdownDryRun",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/upload_all")
	assert.Contains(t, output, `"file_name": "README.md"`)
	assert.Contains(t, output, `"parent_node": "fldcnMarkdownDryRun"`)
	assert.Contains(t, output, `"parent_type": "explorer"`)
	assert.Contains(t, output, `"size": 7`)
}

func TestMarkdownCreateDryRun_FileShowsConcreteSize(t *testing.T) {
	setMarkdownDryRunConfigEnv(t)

	dir := t.TempDir()
	content := "# hi\n"
	require.NoError(t, os.WriteFile(dir+"/note.md", []byte(content), 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+create",
			"--file", "note.md",
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   dir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/upload_all")
	assert.Contains(t, output, `"file": "@note.md"`)
	assert.Contains(t, output, `"size": 5`)
}

func TestMarkdownCreateDryRun_RejectsEmptyContent(t *testing.T) {
	setMarkdownDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+create",
			"--name", "empty.md",
			"--content", "",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	if api := gjson.Get(result.Stdout, "api"); api.IsArray() && len(api.Array()) > 0 {
		t.Fatalf("dry-run api list must be empty when validation fails\nstdout:\n%s", result.Stdout)
	}
	errMsg := gjson.Get(result.Stdout, "error").String()
	assert.Contains(t, errMsg, "empty markdown content is not supported")
}

func TestMarkdownFetchDryRun_OutputFile(t *testing.T) {
	setMarkdownDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+fetch",
			"--file-token", "boxcnMarkdownDryRun",
			"--output", "./copy.md",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/files/boxcnMarkdownDryRun/download")
	assert.Contains(t, output, `"output": "./copy.md"`)
}

func TestMarkdownOverwriteDryRun_ContentFile(t *testing.T) {
	setMarkdownDryRunConfigEnv(t)
	dir := t.TempDir()
	content := "# overwrite test\n"
	require.NoError(t, os.WriteFile(dir+"/input.md", []byte(content), 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+overwrite",
			"--file-token", "boxcnMarkdownDryRun",
			"--content", "@input.md",
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   dir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/drive/v1/metas/batch_query")
	assert.Contains(t, output, "/open-apis/drive/v1/files/upload_all")
	assert.Contains(t, output, `"file_token": "boxcnMarkdownDryRun"`)
	assert.Contains(t, output, `"size": 17`)
}

func TestMarkdownOverwriteDryRun_RejectsEmptyFile(t *testing.T) {
	setMarkdownDryRunConfigEnv(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/empty.md", []byte{}, 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+overwrite",
			"--file-token", "boxcnMarkdownDryRun",
			"--file", "empty.md",
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   dir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	if api := gjson.Get(result.Stdout, "api"); api.IsArray() && len(api.Array()) > 0 {
		t.Fatalf("dry-run api list must be empty when validation fails\nstdout:\n%s", result.Stdout)
	}
	errMsg := gjson.Get(result.Stdout, "error").String()
	assert.Contains(t, errMsg, "empty markdown content is not supported")
}

func setMarkdownDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "markdown_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "markdown_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
