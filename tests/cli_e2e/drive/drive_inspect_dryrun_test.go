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

// --- Happy path: all supported URL types ---

func TestDriveInspectDryRun_DocxURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://xxx.feishu.cn/docx/doxcnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_DocURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://xxx.feishu.cn/doc/doccnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_SheetURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://xxx.feishu.cn/sheets/shtcnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_BitableURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://xxx.feishu.cn/base/bascnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_FileURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://xxx.feishu.cn/file/boxcnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_DoubaoDriveFileURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://feishu.doubao.com/drive/file/boxcnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_FolderURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://xxx.feishu.cn/drive/folder/fldcnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_DoubaoChatDriveFolderURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://feishu.doubao.com/chat/drive/fldcnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_DoubaoDriveShareFolderURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://feishu.doubao.com/drive/shr/fldcnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_MindnoteURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://xxx.feishu.cn/mindnote/mncnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

func TestDriveInspectDryRun_SlidesURL(t *testing.T) {
	setDriveInspectE2EEnv(t)
	result := runInspectDryRun(t, "https://xxx.feishu.cn/slides/slkcnDryRunE2E")
	assertOneStepBatchQuery(t, result)
}

// --- Wiki URL: two-step flow ---

func TestDriveInspectDryRun_WikiURL(t *testing.T) {
	setDriveInspectE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+inspect",
			"--url", "https://xxx.feishu.cn/wiki/wikcnDryRunE2E",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(2), gjson.Get(result.Stdout, "api.#").Int(),
		"expected exactly 2 dry-run API steps for wiki URL, stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/wiki/v2/spaces/get_node",
		gjson.Get(result.Stdout, "api.0.url").String(),
		"expected get_node as first step, stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/drive/v1/metas/batch_query",
		gjson.Get(result.Stdout, "api.1.url").String(),
		"expected batch_query as second step, stdout:\n%s", result.Stdout)
}

// --- Bare token with --type ---

func TestDriveInspectDryRun_BareTokenWithType(t *testing.T) {
	setDriveInspectE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+inspect",
			"--url", "doxcnBareToken",
			"--type", "docx",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	assertOneStepBatchQuery(t, result)
}

// --- Validation errors ---

func TestDriveInspectValidation_EmptyURL(t *testing.T) {
	setDriveInspectE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+inspect",
			"--url", "",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "--url cannot be empty",
		"expected empty URL validation error, stderr:\n%s", result.Stderr)
}

func TestDriveInspectValidation_UnsupportedURL(t *testing.T) {
	setDriveInspectE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+inspect",
			"--url", "https://google.com/some/page",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "unsupported --url",
		"expected unsupported URL validation error, stderr:\n%s", result.Stderr)
}

func TestDriveInspectValidation_BareTokenWithoutType(t *testing.T) {
	setDriveInspectE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+inspect",
			"--url", "doxcnBareToken",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "--type is required when --url is a bare token",
		"expected bare-token-without-type validation error, stderr:\n%s", result.Stderr)
}

func TestDriveInspectValidation_InvalidType(t *testing.T) {
	setDriveInspectE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+inspect",
			"--url", "someToken",
			"--type", "invalid_type",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "invalid_type",
		"expected invalid type validation error, stderr:\n%s", result.Stderr)
}

// --- Helpers ---

func runInspectDryRun(t *testing.T, url string) *clie2e.Result {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+inspect",
			"--url", url,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	return result
}

func assertOneStepBatchQuery(t *testing.T, result *clie2e.Result) {
	t.Helper()

	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int(),
		"expected exactly 1 dry-run API step, stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/drive/v1/metas/batch_query",
		gjson.Get(result.Stdout, "api.0.url").String(),
		"expected batch_query URL, stdout:\n%s", result.Stdout)
}

func setDriveInspectE2EEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "drive_inspect_e2e_app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "drive_inspect_e2e_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
