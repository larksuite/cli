// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_ChartListDryRun pins the request +chart-list emits: a single
// invoke_read call to the get_chart_objects tool with the resolved sheet
// selector. It guards the tool name + endpoint that agents dispatch through.
func TestSheets_ChartListDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+chart-list",
			"--spreadsheet-token", "shtDryRun",
			"--sheet-id", "sheet1",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := clie2e.DryRunData(result.Stdout)
	require.Equal(t, "get_chart_objects", gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_read",
		gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
	// only_thumbnail defaults off — it must not leak into the tool input.
	require.False(t, gjson.Get(out, "tool_input.only_thumbnail").Exists(),
		"only_thumbnail must be absent when the flag is not passed; stdout:\n%s", out)
}

// TestSheets_ChartListOnlyThumbnailDryRun pins that --only-thumbnail flips the
// get_chart_objects tool input's only_thumbnail to true, which is how the
// backend switches from returning the chart snapshot config to returning a
// sized thumbnail image.
func TestSheets_ChartListOnlyThumbnailDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+chart-list",
			"--spreadsheet-token", "shtDryRun",
			"--sheet-id", "sheet1",
			"--only-thumbnail",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := clie2e.DryRunData(result.Stdout)
	require.Equal(t, "get_chart_objects", gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)
	require.True(t, gjson.Get(out, "tool_input.only_thumbnail").Bool(),
		"--only-thumbnail must set only_thumbnail=true on the tool input; stdout:\n%s", out)
}
