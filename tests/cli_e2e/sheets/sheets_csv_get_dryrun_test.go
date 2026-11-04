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

func TestSheets_CsvGetCitationDryRun(t *testing.T) {
	for _, gate := range []string{"1", "0"} {
		t.Run(gate, func(t *testing.T) {
			setSheetsDryRunEnv(t)
			t.Setenv("LARKSUITE_CLI_CITATION", gate)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"sheets", "+csv-get", "--url", "https://example.feishu.cn/sheets/shtCsvDryRun",
					"--sheet-id", "sheet1", "--range", "A1:B2", "--max-chars", "25000", "--dry-run",
				},
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Len(t, clie2e.DryRunGet(result.Stdout, "api").Array(), 1)
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtCsvDryRun/tools/invoke_read", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, "get_range_as_csv", clie2e.DryRunGet(result.Stdout, "api.0.body.tool_name").String())
			input := clie2e.DryRunGet(result.Stdout, "api.0.body.input").String()
			require.Equal(t, "shtCsvDryRun", gjson.Get(input, "excel_id").String())
			require.Equal(t, "sheet1", gjson.Get(input, "sheet_id").String())
			require.Equal(t, "A1:B2", gjson.Get(input, "range").String())
			require.Equal(t, int64(25000), gjson.Get(input, "max_chars").Int())
			require.Equal(t, int64(1000000000), gjson.Get(input, "max_rows").Int())
			require.False(t, gjson.Get(result.Stdout, "citations").Exists())
		})
	}
}
