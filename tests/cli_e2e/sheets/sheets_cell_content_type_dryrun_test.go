// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSheets_CellContentTypeDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	const payload = `[[{"type":"string","value":"00123","cell_styles":{"font":{"bold":1,"italic":0e0,"underline":1.0}}},{"type":"number","value":0},{"type":"boolean","value":false},{"type":"formula","value":"=SUM(1,2)"},{"rich_text":[{"type":"mention","text":"x","mention_token":"t","mention_type":0.0}]}]]`
	path := filepath.Join(t.TempDir(), "cells.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	for _, tc := range []struct{ name, flag, stdin string }{
		{"inline", payload, ""}, {"file", "@" + path, ""}, {"stdin", "-", payload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"sheets", "+cells-set", "--spreadsheet-token", "shtDryRun", "--sheet-name", "S",
					"--range", "A1:E1", "--cells", tc.flag, "--allow-overwrite=false", "--dry-run"},
				Stdin: []byte(tc.stdin), DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, "set_cell_range", clie2e.DryRunGet(result.Stdout, "api.0.body.tool_name").String())
			input := clie2e.DryRunGet(result.Stdout, "api.0.body.input").String()
			require.Equal(t, "S", gjson.Get(input, "sheet_name").String())
			require.Equal(t, "A1:E1", gjson.Get(input, "range").String())
			require.Equal(t, false, gjson.Get(input, "allow_overwrite").Value())
			require.Equal(t, "00123", gjson.Get(input, "cells.0.0.value").Value())
			require.Equal(t, "bold", gjson.Get(input, "cells.0.0.cell_styles.font_weight").String())
			require.Equal(t, "normal", gjson.Get(input, "cells.0.0.cell_styles.font_style").String())
			require.Equal(t, "underline", gjson.Get(input, "cells.0.0.cell_styles.font_line").String())
			require.Equal(t, "0.0", gjson.Get(input, "cells.0.4.rich_text.0.mention_type").Raw)
			require.Equal(t, float64(0), gjson.Get(input, "cells.0.1.value").Value())
			require.Equal(t, false, gjson.Get(input, "cells.0.2.value").Value())
			require.Equal(t, "=SUM(1,2)", gjson.Get(input, "cells.0.3.formula").Value())
			require.False(t, gjson.Get(input, "cells.0.3.value").Exists())
			for _, cell := range gjson.Get(input, "cells.0").Array() {
				require.False(t, cell.Get("type").Exists())
			}
		})
	}
}
