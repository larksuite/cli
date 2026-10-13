// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCellsSetNumericFontSwitchesAcrossCarriers(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, counts := range []struct{ bold, italic, underline, weight, style, line string }{
		{"1", "0", "1", "bold", "normal", "underline"},
		{"0.0", "1e0", "0e0", "normal", "italic", "none"},
	} {
		font := `{"bold":` + counts.bold + `,"italic":` + counts.italic + `,"underline":` + counts.underline + `}`
		cell := `{"value":9007199254740993,"cell_styles":{"font":` + font + `}}`
		for _, tc := range []struct {
			name, path string
			args       []string
		}{
			{"dense", "cells.0.0", []string{"--range", "A1", "--cells", `[[` + cell + `]]`}},
			{"writes", "operations.0.input.cells.0.0", []string{"--writes", `[{"range":"A1","cells":[[` + cell + `]]}]`}},
			{"sparse", "operations.0.input.cells.0.0", []string{"--cells", `[{"cell":"A1","value":9007199254740993,"cell_styles":{"font":` + font + `}}]`}},
		} {
			t.Run(tc.name+"/"+font, func(t *testing.T) {
				args := append([]string{"--url", testURL, "--sheet-id", testSheetID}, tc.args...)
				api := parseDryRunAPI(t, CellsSet, args)
				preview := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
				write := toolOutputStub(testToken, "write", `{}`)
				_, err := runShortcutWithStubs(t, CellsSet, args, write)
				require.NoError(t, err)
				require.Len(t, write.CapturedBodies, 1)
				for _, input := range []string{preview, gjson.GetBytes(write.CapturedBody, "input").String()} {
					leaf := gjson.Get(input, tc.path)
					require.Equal(t, "9007199254740993", leaf.Get("value").Raw)
					require.Equal(t, counts.weight, leaf.Get("cell_styles.font_weight").String())
					require.Equal(t, counts.style, leaf.Get("cell_styles.font_style").String())
					require.Equal(t, counts.line, leaf.Get("cell_styles.font_line").String())
					require.False(t, leaf.Get("cell_styles.font").Exists())
				}
			})
		}
	}
}

func TestCellsSetNumericMentionEnumsKeepEquivalentSpellings(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, number := range []string{"0.0", "0e0", "22.0", "2.2e1"} {
		cell := `{"rich_text":[{"type":"mention","text":"x","mention_token":"t","mention_type":` + number + `}]}`
		for _, tc := range []struct {
			name, path string
			args       []string
		}{
			{"dense", "cells.0.0", []string{"--range", "A1", "--cells", `[[` + cell + `]]`}},
			{"writes", "operations.0.input.cells.0.0", []string{"--writes", `[{"range":"A1","cells":[[` + cell + `]]}]`}},
		} {
			t.Run(tc.name+"/"+number, func(t *testing.T) {
				args := append([]string{"--url", testURL, "--sheet-id", testSheetID}, tc.args...)
				api := parseDryRunAPI(t, CellsSet, args)
				preview := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
				write := toolOutputStub(testToken, "write", `{}`)
				_, err := runShortcutWithStubs(t, CellsSet, args, write)
				require.NoError(t, err)
				require.Len(t, write.CapturedBodies, 1)
				for _, input := range []string{preview, gjson.GetBytes(write.CapturedBody, "input").String()} {
					require.Equal(t, number, gjson.Get(input, tc.path+".rich_text.0.mention_type").Raw)
				}
			})
		}
	}
}

func TestCellsSetNumericStylesAndEnumsRejectDifferentValues(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, cell := range []string{
		`{"value":"x","cell_styles":{"font":{"bold":2}}}`,
		`{"value":"x","cell_styles":{"font":{"italic":0.5}}}`,
		`{"value":"x","cell_styles":{"font":{"underline":1.0000000000000000001}}}`,
		`{"rich_text":[{"type":"mention","text":"x","mention_token":"t","mention_type":6.0}]}`,
		`{"rich_text":[{"type":"mention","text":"x","mention_token":"t","mention_type":22.000000000000000001}]}`,
	} {
		t.Run(cell, func(t *testing.T) {
			write := toolOutputStub(testToken, "write", `{}`)
			write.Optional = true
			out, err := runShortcutWithStubs(t, CellsSet, []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1", "--cells", `[[` + cell + `]]`}, write)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, "--cells", validation.Param)
			require.Empty(t, write.CapturedBodies)
			require.Empty(t, out)
		})
	}
}

func TestCellsSetEquivalentNumericAliasesKeepExactValues(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	api := parseDryRunAPI(t, CellsSet, []string{"--url", testURL, "--sheet-id", testSheetID, "--writes",
		`[{"range":"A1","cells":[[{"value":9007199254740993}]],"values":[[{"value":9007199254740993.0}]]}]`,
	})
	input := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
	require.Equal(t, "9007199254740993", gjson.Get(input, "operations.0.input.cells.0.0.value").Raw)
	write := toolOutputStub(testToken, "write", `{}`)
	write.Optional = true
	out, err := runShortcutWithStubs(t, CellsSet, []string{"--url", testURL, "--sheet-id", testSheetID, "--writes",
		`[{"range":"A1","cells":[[{"value":9007199254740993}]],"values":[[{"value":9007199254740992}]]}]`,
	}, write)
	var validation *errs.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, "--writes", validation.Param)
	require.Empty(t, write.CapturedBodies)
	require.Empty(t, out)
}
