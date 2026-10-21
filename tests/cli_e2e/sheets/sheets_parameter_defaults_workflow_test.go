// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Each input is compared with an explicit canonical control in separate cells.
// The API owns omitted attributes, so record its default weight instead of
// guessing one. Explicit weight/color refinements start with visible borders,
// making a dropped refinement distinguishable from a successful write.
// The backend replaces a selected side rather than patching its fields:
// mapBorderLineStyle in ai-tools/tools-imp/set-cell-range.ts maps omitted style
// to thin (read back as solid/thin), even with weight:medium. Dry-run tests pin
// the requested medium; these live cases pin the server's canonical result.
func TestSheets_BorderParameterDefaultsWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)
	spreadsheetToken, sheetName := newParameterDefaultsSheet(t, ctx, "border-defaults")

	cases := []struct {
		name, border, control, weight, color, style string
		attributes                                  []string
		seedBorder                                  bool
		flag                                        string
		selectedSides                               []string
	}{
		{
			name:   "empty weight matches omitted weight for a solid border",
			border: `{"left":{"style":"solid","weight":""}}`, control: `{"left":{"style":"solid"}}`, style: "solid",
		},
		{
			name:   "empty JSON and flag weights retain line and color",
			border: `{"left":{"style":"dashed","color":"#123456","weight":""}}`, control: `{"left":{"style":"dashed","color":"#123456"}}`,
			attributes: []string{"--border-weight", ""}, style: "dashed", color: "#123456",
		},
		{
			name:   "width survives empty weight",
			border: `{"left":{"style":"solid","width":2,"weight":""}}`, control: `{"left":{"style":"solid","weight":"medium"}}`,
			style: "solid", weight: "medium",
		},
		{
			name: "empty side with weight matches server default without style", border: `{"left":{}}`, control: `{"left":{"weight":"medium"}}`,
			attributes: []string{"--border-weight", "medium"}, style: "solid", weight: "thin", seedBorder: true,
		},
		{
			name: "empty side receives color with server default line", border: `{"left":{}}`, control: `{"left":{"color":"#123456"}}`,
			attributes: []string{"--border-color", "#123456"}, style: "solid", weight: "thin", color: "#123456", seedBorder: true,
		},
		{
			name: "empty side with both attributes matches server default without style", border: `{"left":{}}`, control: `{"left":{"weight":"medium","color":"#123456"}}`,
			attributes: []string{"--border-weight", "medium", "--border-color", "#123456"}, style: "solid", weight: "thin", color: "#123456", seedBorder: true,
		},
		{
			name: "border alias empty weight Regression writes both selected sides",
			flag: "style", border: `{"border":{"left":{"weight":""},"right":{"style":"solid"}}}`,
			control:    `{"left":{"color":"#123456"},"right":{"style":"solid","color":"#123456"}}`,
			attributes: []string{"--border-color", "#123456"}, style: "solid", weight: "thin", color: "#123456",
			seedBorder: true, selectedSides: []string{"left", "right"},
		},
	}
	for commandIndex, command := range []string{"+cells-set-style", "+cells-batch-set-style", "+batch-update"} {
		for caseIndex, tc := range cases {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				selectedSides := tc.selectedSides
				if len(selectedSides) == 0 {
					selectedSides = []string{"left"}
				}
				// The gap prevents adjacent cells from sharing a border edge.
				row := 2 + 2*(commandIndex*len(cases)+caseIndex)
				target, control := fmt.Sprintf("B%d", row), fmt.Sprintf("D%d", row)
				rng := target + ":" + control
				cells := []map[string]any{{"value": "target"}, {"value": "gap"}, {"value": "control"}}
				// Distinct nonempty sentinels expose both accidental clearing and
				// copying between the target and control's unselected sides.
				// Solid lines preserve all three weights in backend serialization;
				// dotted/dashed combinations can normalize to a different weight.
				sentinels := map[int]map[string]map[string]string{
					0: {
						"top":    {"style": "solid", "weight": "thin", "color": "#112233"},
						"bottom": {"style": "solid", "weight": "medium", "color": "#223344"},
						"right":  {"style": "solid", "weight": "thick", "color": "#334455"},
					},
					2: {
						"top":    {"style": "solid", "weight": "medium", "color": "#445566"},
						"bottom": {"style": "solid", "weight": "thick", "color": "#556677"},
						"right":  {"style": "solid", "weight": "thin", "color": "#667788"},
					},
				}
				for _, index := range []int{0, 2} {
					borders := map[string]any{}
					for side, spec := range sentinels[index] {
						borders[side] = spec
					}
					if tc.seedBorder {
						borders["left"] = map[string]any{
							"style": "solid", "weight": "thick", "color": "#654321",
						}
					}
					cells[index]["border_styles"] = borders
				}
				seed, err := json.Marshal([]any{cells})
				require.NoError(t, err)
				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{"sheets", "+cells-set", "--spreadsheet-token", spreadsheetToken, "--sheet-name", sheetName,
						"--range", rng, "--cells", string(seed)}, DefaultAs: "bot",
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)

				before, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken, "--sheet-name", sheetName,
						"--range", rng, "--include", "value,style"}, DefaultAs: "bot",
				})
				require.NoError(t, err)
				before.AssertExitCode(t, 0)
				before.AssertStdoutStatus(t, true)
				require.Equal(t, true, gjson.Get(before.Stdout, "data.complete").Value())
				require.Equal(t, false, gjson.Get(before.Stdout, "data.has_more").Value())
				beforeCells := gjson.Get(before.Stdout, "data.ranges.0.cells.0").Array()
				require.Len(t, beforeCells, 3)
				for _, index := range []int{0, 2} {
					for side, spec := range sentinels[index] {
						got := beforeCells[index].Get("border_styles." + side)
						require.True(t, got.IsObject(), "cell %d must start with a nonempty %s border", index, side)
						require.Equal(t, spec["style"], got.Get("style").String())
						require.Equal(t, spec["weight"], got.Get("weight").String())
						require.Equal(t, spec["color"], strings.ToLower(got.Get("color").String()))
					}
					if tc.seedBorder {
						side := beforeCells[index].Get("border_styles.left")
						require.Equal(t, "solid", side.Get("style").String())
						require.Equal(t, "thick", side.Get("weight").String())
						require.Equal(t, "#654321", strings.ToLower(side.Get("color").String()))
					}
				}

				controlArgs := borderDefaultsArgs(t, "+cells-set-style", sheetName, control, tc.control)
				result, err = clie2e.RunCmd(ctx, clie2e.Request{
					Args: append(controlArgs, "--spreadsheet-token", spreadsheetToken), DefaultAs: "bot",
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)
				flag := tc.flag
				if flag == "" {
					flag = "border-styles"
				}
				args := borderDefaultsInputArgs(t, command, sheetName, target, flag, tc.border, tc.attributes...)
				result, err = clie2e.RunCmd(ctx, clie2e.Request{
					Args: append(args, "--spreadsheet-token", spreadsheetToken), DefaultAs: "bot", Yes: command == "+batch-update",
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)

				result, err = clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken, "--sheet-name", sheetName,
						"--range", rng, "--include", "value,style"}, DefaultAs: "bot",
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)
				require.Equal(t, true, gjson.Get(result.Stdout, "data.complete").Value())
				require.Equal(t, false, gjson.Get(result.Stdout, "data.has_more").Value())
				require.Equal(t, int64(3), gjson.Get(result.Stdout, "data.returned_cell_count").Int())
				readCells := gjson.Get(result.Stdout, "data.ranges.0.cells.0").Array()
				require.Len(t, readCells, 3)
				for i, want := range []string{"target", "gap", "control"} {
					require.Equal(t, want, readCells[i].Get("value").String())
				}
				for _, side := range selectedSides {
					got, want := readCells[0].Get("border_styles."+side), readCells[2].Get("border_styles."+side)
					require.True(t, want.IsObject(), "canonical control must persist its %s border", side)
					require.True(t, got.IsObject(), "target must persist its selected %s border", side)
					require.Equal(t, want.Exists(), got.Exists(), "selected side presence must match its canonical control")
					require.Equal(t, want.Value(), got.Value(), "selected side must match its canonical control")
					if tc.weight == "" {
						t.Logf("default weight: control present=%t value=%q; empty weight present=%t value=%q",
							want.Get("weight").Exists(), want.Get("weight").String(), got.Get("weight").Exists(), got.Get("weight").String())
					} else {
						require.Equal(t, tc.weight, got.Get("weight").String())
					}
					if tc.style != "" {
						require.Equal(t, tc.style, got.Get("style").String())
					}
					if tc.color != "" {
						require.Equal(t, tc.color, strings.ToLower(got.Get("color").String()))
					}
				}
				var controlBorder map[string]any
				require.NoError(t, json.Unmarshal([]byte(tc.control), &controlBorder))
				for _, index := range []int{0, 2} {
					for _, side := range []string{"top", "bottom", "right"} {
						if _, selected := controlBorder[side]; selected {
							continue
						}
						path := "border_styles." + side
						require.True(t, readCells[index].Get(path).IsObject(), "cell %d lost its %s border", index, side)
						require.Equal(t, beforeCells[index].Get(path).Value(), readCells[index].Get(path).Value(),
							"cell %d must retain its original unselected %s border", index, side)
					}
				}
			})
		}
	}
}

func TestSheets_ResizeTypeAliasesWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	spreadsheetToken, sheetName := newParameterDefaultsSheet(t, ctx, "resize-aliases")
	for i, command := range []string{"+styles-put", "+table-put"} {
		t.Run(command, func(t *testing.T) {
			// Different sizes on the second carrier prevent its write from
			// passing solely on the state persisted by the first carrier.
			offset := i * 8
			styles, err := json.Marshal(map[string]any{"styles": []any{map[string]any{
				"name": sheetName,
				"row_sizes": []any{
					map[string]any{"range": "7:7", "type": "rows", "size": 44 + offset},
					map[string]any{"range": "8:8", "type": "ROWS", "size": 52 + offset},
					map[string]any{"range": "9:9", "type": "px", "size": 60 + offset},
				},
				"col_sizes": []any{
					map[string]any{"range": "D:D", "type": "width", "size": 96 + offset},
					map[string]any{"range": "E:E", "type": "px", "size": 144 + offset},
				},
			}}})
			require.NoError(t, err)
			args := []string{"sheets", command, "--spreadsheet-token", spreadsheetToken, "--styles", string(styles)}
			if command == "+table-put" {
				payload, err := json.Marshal(map[string]any{"sheets": []any{map[string]any{
					"name": sheetName, "header": false, "columns": []string{"marker"}, "data": [][]string{{"resize-marker"}},
				}}})
				require.NoError(t, err)
				args = append(args, "--sheets", string(payload))
			}
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			result, err = clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"sheets", "+sheet-info", "--spreadsheet-token", spreadsheetToken, "--sheet-name", sheetName,
					"--range", "A7:F9", "--include", "row_heights,col_widths"}, DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			require.True(t, gjson.Get(result.Stdout, "data.row_heights").IsArray())
			require.True(t, gjson.Get(result.Stdout, "data.column_widths").IsArray())
			for path, want := range map[string]int{
				`data.row_heights.#(rows=="7:7").height`:  44 + offset,
				`data.row_heights.#(rows=="8:8").height`:  52 + offset,
				`data.row_heights.#(rows=="9:9").height`:  60 + offset,
				`data.column_widths.#(cols=="D:D").width`: 96 + offset,
				`data.column_widths.#(cols=="E:E").width`: 144 + offset,
			} {
				value := gjson.Get(result.Stdout, path)
				require.True(t, value.Exists(), "missing size metadata at %s", path)
				require.Equal(t, int64(want), value.Int(), "size metadata at %s", path)
			}
			if command == "+table-put" {
				result, err = clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken, "--sheet-name", sheetName,
						"--range", "A1", "--include", "value"}, DefaultAs: "bot",
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)
				require.Equal(t, "resize-marker", gjson.Get(result.Stdout, "data.ranges.0.cells.0.0.value").String())
			}
		})
	}
}

func newParameterDefaultsSheet(t *testing.T, ctx context.Context, name string) (string, string) {
	t.Helper()
	suffix := clie2e.GenerateSuffix()
	// createSpreadsheet registers workbook and folder teardown on the parent,
	// including when any subsequent setup or read-after-write assertion fails.
	token := createSpreadsheet(t, t, ctx, "lark-cli-e2e-"+name+"-"+suffix, "bot")
	sheetName := name + "-" + suffix
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"sheets", "+sheet-create", "--spreadsheet-token", token, "--title", sheetName}, DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.NotEmpty(t, gjson.Get(result.Stdout, "data.sheet_id").String())
	return token, sheetName
}
