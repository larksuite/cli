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

// TestDriveInfoDryRun_DocxURL asserts that drive +info --dry-run with a docx URL
// plans a single batch_query API call (no wiki unwrap needed).
func TestDriveInfoDryRun_DocxURL(t *testing.T) {
	setDriveInfoE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+info",
			"--url", "https://xxx.feishu.cn/docx/doxcnDryRunE2E",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	// Should plan a single batch_query call (no wiki step).
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int(),
		"expected exactly 1 dry-run API step for docx URL, stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/drive/v1/metas/batch_query",
		gjson.Get(result.Stdout, "api.0.url").String(),
		"expected batch_query URL, stdout:\n%s", result.Stdout)
}

// TestDriveInfoDryRun_WikiURL asserts that drive +info --dry-run with a wiki URL
// plans a two-step flow: get_node then batch_query.
func TestDriveInfoDryRun_WikiURL(t *testing.T) {
	setDriveInfoE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+info",
			"--url", "https://xxx.feishu.cn/wiki/wikcnDryRunE2E",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	// Should plan two steps: get_node + batch_query.
	require.Equal(t, int64(2), gjson.Get(result.Stdout, "api.#").Int(),
		"expected exactly 2 dry-run API steps for wiki URL, stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/wiki/v2/spaces/get_node",
		gjson.Get(result.Stdout, "api.0.url").String(),
		"expected get_node as first step, stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/drive/v1/metas/batch_query",
		gjson.Get(result.Stdout, "api.1.url").String(),
		"expected batch_query as second step, stdout:\n%s", result.Stdout)
}

// TestDriveInfoDryRun_BareTokenWithType asserts that drive +info --dry-run with a
// bare token and --type plans a single batch_query call.
func TestDriveInfoDryRun_BareTokenWithType(t *testing.T) {
	setDriveInfoE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+info",
			"--url", "doxcnBareToken",
			"--type", "docx",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(1), gjson.Get(result.Stdout, "api.#").Int(),
		"expected exactly 1 dry-run API step for bare token, stdout:\n%s", result.Stdout)
	require.Equal(t, "/open-apis/drive/v1/metas/batch_query",
		gjson.Get(result.Stdout, "api.0.url").String(),
		"expected batch_query URL, stdout:\n%s", result.Stdout)
}

func setDriveInfoE2EEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "drive_info_e2e_app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "drive_info_e2e_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
