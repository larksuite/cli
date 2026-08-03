// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestSheets_CSVPutDryRunAcceptsTokenAlias(t *testing.T) {
	setSheetsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+csv-put",
			"--token", "shtDryRun",
			"--sheet-id", "shDryRun",
			"--csv", "a,b\n1,2",
			"--start-cell", "C4",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "shtDryRun", clie2e.DryRunGet(result.Stdout, "tool_input.excel_id").String(), result.Stdout)
	require.Equal(t, "C4", clie2e.DryRunGet(result.Stdout, "tool_input.start_cell").String(), result.Stdout)
	require.Equal(t, "set_range_from_csv", clie2e.DryRunGet(result.Stdout, "tool_name").String(), result.Stdout)
}
