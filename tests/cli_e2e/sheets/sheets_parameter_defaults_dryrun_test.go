// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the compiled command's parsing and normalization, including the
// separate flag and batch-suboperation paths that helper tests cannot cover.
func TestSheets_BorderParameterDefaultsDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, command := range []string{"+cells-set-style", "+cells-batch-set-style", "+batch-update"} {
		for _, tc := range []struct {
			name, border, wantSide string
			attributes             []string
		}{
			{"empty side is rejected", `{"left":{}}`, "", nil},
			{"empty weight without effective attributes is rejected", `{"left":{"weight":""}}`, "", nil},
			{"empty weight flag without effective attributes is rejected", `{"left":{}}`, "", []string{"--border-weight", ""}},
			{"empty weight preserves line and color", `{"left":{"style":"dashed","color":"#123456","weight":""}}`, `{"style":"dashed","color":"#123456"}`, nil},
			{"empty weight flag preserves line", `{"left":{"style":"dashed"}}`, `{"style":"dashed"}`, []string{"--border-weight", ""}},
			{"width survives empty weight", `{"left":{"style":"dashed","width":2,"weight":""}}`, `{"style":"dashed","weight":"medium"}`, nil},
			{"empty side with weight flag", `{"left":{}}`, `{"weight":"medium"}`, []string{"--border-weight", "medium"}},
			{"empty side with color flag", `{"left":{}}`, `{"color":"#123456"}`, []string{"--border-color", "#123456"}},
			{"empty side with both attributes", `{"left":{}}`, `{"weight":"medium","color":"#123456"}`, []string{"--border-weight", "medium", "--border-color", "#123456"}},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				args := borderDefaultsArgs(t, command, "S", "B2:B2", tc.border, tc.attributes...)
				args = append(args, "--spreadsheet-token", "shtDryRun", "--dry-run")
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
				require.NoError(t, err)
				if tc.wantSide == "" {
					result.AssertExitCode(t, 2)
					require.Empty(t, result.Stdout)
					require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
					require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
					param := "--border-styles"
					if command == "+batch-update" {
						param = "--operations"
					}
					require.Equal(t, param, gjson.Get(result.Stderr, "error.param").String())
					return
				}
				result.AssertExitCode(t, 0)
				border := assertBorderDefaultsDryRunRequest(t, command, result.Stdout)
				require.Len(t, border.Map(), 1, result.Stdout)
				require.True(t, border.Get("left").IsObject(), result.Stdout)
				require.JSONEq(t, tc.wantSide, border.Get("left").Raw)
			})
		}
	}
}

func TestSheets_BorderAliasEmptyWeightDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, command := range []string{"+cells-set-style", "+cells-batch-set-style", "+batch-update"} {
		for _, tc := range []struct {
			name, style, want, errorParam string
			attributes                    []string
		}{
			{
				name: "border mixed sides receive color", style: `{"border":{"left":{"weight":""},"right":{"style":"solid"}}}`,
				attributes: []string{"--border-color", "#123456"},
				want:       `{"left":{"color":"#123456"},"right":{"style":"solid","color":"#123456"}}`,
			},
			{
				name: "borders whitespace weight receives weight", style: `{"borders":{"left":{"weight":" \t"},"right":{"style":"solid"}}}`,
				attributes: []string{"--border-weight", "medium"},
				want:       `{"left":{"weight":"medium"},"right":{"style":"solid","weight":"medium"}}`,
			},
			{
				name: "per side aliases receive both attributes", style: `{"border_left":{"weight":""},"right_border":{"style":"solid"}}`,
				attributes: []string{"--border-weight", "medium", "--border-color", "#123456"},
				want:       `{"left":{"weight":"medium","color":"#123456"},"right":{"style":"solid","weight":"medium","color":"#123456"}}`,
			},
			{
				name: "empty alias retains canonical attributes", style: `{"border":{"left":{"weight":""}},"border_styles":{"left":{"style":"dashed","weight":"thick","color":"#123456"}}}`,
				want: `{"left":{"style":"dashed","weight":"thick","color":"#123456"}}`,
			},
			{
				name: "canonical and alias attributes merge", style: `{"left_border":{"weight":" \t","color":"#123456"},"border_styles":{"left":{"style":"solid","weight":"medium"}}}`,
				want: `{"left":{"style":"solid","weight":"medium","color":"#123456"}}`,
			},
			{
				name: "mixed sides still reject unfilled empty weight", style: `{"border":{"left":{"weight":""},"right":{"style":"solid"}}}`,
				errorParam: "--border-styles",
			},
			{
				name: "empty flag still rejects whitespace weight", style: `{"borders":{"left":{"weight":" \t"},"right":{"style":"solid"}}}`,
				attributes: []string{"--border-weight", ""}, errorParam: "--border-styles",
			},
			{
				name: "empty alias conflicts with scalar canonical side", style: `{"border_left":{"weight":""},"border_styles":{"left":"solid"}}`,
				errorParam: "--style",
			},
			{
				name: "empty alias conflicts with null canonical side", style: `{"left_border":{},"border_styles":{"left":null}}`,
				errorParam: "--style",
			},
			{
				name: "canonical attribute collision is rejected", style: `{"border":{"left":{"weight":"","color":"#123456"}},"border_styles":{"left":{"color":"#654321"}}}`,
				errorParam: "--style",
			},
			{
				name: "independent attribute collision is rejected", style: `{"border":{"left":{"weight":""},"right":{"color":"#654321"}}}`,
				attributes: []string{"--border-color", "#123456"}, errorParam: "--border-color",
			},
			{
				name: "null weight remains invalid", style: `{"border":{"left":{"weight":null},"right":{"style":"solid"}}}`,
				attributes: []string{"--border-color", "#123456"}, errorParam: "--style",
			},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				args := borderDefaultsInputArgs(t, command, "S", "B2:B2", "style", tc.style, tc.attributes...)
				args = append(args, "--spreadsheet-token", "shtDryRun", "--dry-run")
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
				require.NoError(t, err)
				if tc.errorParam != "" {
					result.AssertExitCode(t, 2)
					require.Empty(t, result.Stdout)
					require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
					require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
					param := tc.errorParam
					if command == "+batch-update" {
						param = "--operations"
					}
					require.Equal(t, param, gjson.Get(result.Stderr, "error.param").String())
					return
				}
				result.AssertExitCode(t, 0)
				border := assertBorderDefaultsDryRunRequest(t, command, result.Stdout)
				require.JSONEq(t, tc.want, border.Raw)
			})
		}
	}
}

func assertBorderDefaultsDryRunRequest(t *testing.T, command, stdout string) gjson.Result {
	t.Helper()
	calls := clie2e.DryRunGet(stdout, "api").Array()
	require.Len(t, calls, 1, stdout)
	assertDefaultsWriteCall(t, calls[0])
	input := gjson.Parse(calls[0].Get("body.input").String())
	if command == "+cells-set-style" {
		require.Equal(t, "set_cell_range", calls[0].Get("body.tool_name").String())
	} else {
		require.Equal(t, "batch_update", calls[0].Get("body.tool_name").String())
		ops := input.Get("operations").Array()
		require.Len(t, ops, 1, input.Raw)
		require.Equal(t, "set_cell_range", ops[0].Get("tool_name").String())
		input = ops[0].Get("input")
	}
	require.Equal(t, "shtDryRun", input.Get("excel_id").String())
	require.Equal(t, "S", input.Get("sheet_name").String())
	require.Equal(t, "B2:B2", input.Get("range").String())
	require.Len(t, input.Get("cells").Array(), 1)
	require.Len(t, input.Get("cells.0").Array(), 1)
	cell := input.Get("cells.0.0")
	require.False(t, cell.Get("value").Exists(), input.Raw)
	require.False(t, cell.Get("formula").Exists(), input.Raw)
	return cell.Get("border_styles")
}

// Dimension type aliases belong to the declarative --styles vocabulary. Cover
// both its batched +styles-put request and +table-put's individual resize calls.
func TestSheets_ResizeTypeAliasesDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)
	for _, command := range []string{"+styles-put", "+table-put"} {
		for _, tc := range []struct {
			name, section, target, typ, sizeKey string
			invalid                             bool
		}{
			{"rows", "row_sizes", "7:7", "rows", "size", false},
			{"ROWS", "row_sizes", "7:7", "ROWS", "size", false},
			{"width", "col_sizes", "D:D", "width", "size", false},
			{"px rows", "row_sizes", "7:7", "px", "size", false},
			{"px columns", "col_sizes", "D:D", "px", "size", false},
			{"rows with height", "row_sizes", "7:7", "rows", "height", false},
			{"width with width", "col_sizes", "D:D", "width", "width", false},
			{"rows missing size", "row_sizes", "7:7", "rows", "", true},
			{"ROWS missing size", "row_sizes", "7:7", "ROWS", "", true},
			{"width missing size", "col_sizes", "D:D", "width", "", true},
			{"px missing size", "row_sizes", "7:7", "px", "", true},
			{"row type on columns", "col_sizes", "D:D", "rows", "size", true},
			{"column type on rows", "row_sizes", "7:7", "width", "size", true},
			{"column range on rows", "row_sizes", "D:D", "rows", "size", true},
			{"row range on columns", "col_sizes", "7:7", "width", "size", true},
			{"width size on rows", "row_sizes", "7:7", "rows", "width", true},
			{"height size on columns", "col_sizes", "D:D", "width", "height", true},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				entry := map[string]any{"range": tc.target, "type": tc.typ}
				if tc.sizeKey != "" {
					entry[tc.sizeKey] = 42
				}
				styles, err := json.Marshal(map[string]any{"styles": []any{map[string]any{
					"name": "S", tc.section: []any{entry},
				}}})
				require.NoError(t, err)
				args := []string{"sheets", command, "--spreadsheet-token", "shtDryRun", "--styles", string(styles), "--dry-run"}
				if command == "+table-put" {
					args = append(args, "--sheets", `{"sheets":[{"name":"S","header":false,"columns":["marker"],"data":[["keep"]]}]}`)
				}
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
				require.NoError(t, err)
				if tc.invalid {
					result.AssertExitCode(t, 2)
					require.Empty(t, result.Stdout)
					require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
					require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
					require.Equal(t, "--styles", gjson.Get(result.Stderr, "error.param").String())
					return
				}
				result.AssertExitCode(t, 0)
				calls := clie2e.DryRunGet(result.Stdout, "api").Array()
				var input gjson.Result
				if command == "+styles-put" {
					require.Len(t, calls, 1, result.Stdout)
					require.Equal(t, "batch_update", calls[0].Get("body.tool_name").String())
					ops := gjson.Get(calls[0].Get("body.input").String(), "operations").Array()
					require.Len(t, ops, 1, result.Stdout)
					require.Equal(t, "resize_range", ops[0].Get("tool_name").String())
					input = ops[0].Get("input")
				} else {
					require.Len(t, calls, 2, result.Stdout)
					require.Equal(t, "set_cell_range", calls[0].Get("body.tool_name").String())
					require.Equal(t, "keep", gjson.Get(calls[0].Get("body.input").String(), "cells.0.0.value").String())
					require.Equal(t, "resize_range", calls[1].Get("body.tool_name").String())
					input = gjson.Parse(calls[1].Get("body.input").String())
				}
				for _, call := range calls {
					assertDefaultsWriteCall(t, call)
				}
				require.Equal(t, "shtDryRun", input.Get("excel_id").String())
				require.Equal(t, "S", input.Get("sheet_name").String())
				require.Equal(t, tc.target, input.Get("range").String())
				field, other := "resize_height", "resize_width"
				if tc.section == "col_sizes" {
					field, other = other, field
				}
				require.JSONEq(t, `{"type":"pixel","value":42}`, input.Get(field).Raw)
				require.False(t, input.Get(other).Exists(), input.Raw)
			})
		}
	}
}

func assertDefaultsWriteCall(t *testing.T, call gjson.Result) {
	t.Helper()
	require.Equal(t, "POST", call.Get("method").String())
	require.Equal(t, "/open-apis/sheet_ai/v2/spreadsheets/shtDryRun/tools/invoke_write", call.Get("url").String())
}

// Keep the same caller payload across standalone, range-batch and sub-op entry
// points. JSON flags remain command arguments; these shortcuts have no --data.
func borderDefaultsArgs(t *testing.T, command, sheetName, target, border string, attributes ...string) []string {
	t.Helper()
	return borderDefaultsInputArgs(t, command, sheetName, target, "border-styles", border, attributes...)
}

func borderDefaultsInputArgs(t *testing.T, command, sheetName, target, flag, value string, attributes ...string) []string {
	t.Helper()
	args := []string{"sheets", command}
	switch command {
	case "+cells-set-style":
		args = append(args, "--sheet-name", sheetName, "--range", target, "--"+flag, value)
	case "+cells-batch-set-style":
		ranges, err := json.Marshal([]string{sheetName + "!" + target})
		require.NoError(t, err)
		args = append(args, "--ranges", string(ranges), "--"+flag, value)
	case "+batch-update":
		var spec map[string]any
		require.NoError(t, json.Unmarshal([]byte(value), &spec))
		input := map[string]any{"sheet_name": sheetName, "range": target, strings.ReplaceAll(flag, "-", "_"): spec}
		for i := 0; i < len(attributes); i += 2 {
			key := strings.ReplaceAll(strings.TrimPrefix(attributes[i], "--"), "-", "_")
			input[key] = attributes[i+1]
		}
		ops, err := json.Marshal([]any{map[string]any{"shortcut": "+cells-set-style", "input": input}})
		require.NoError(t, err)
		return append(args, "--operations", string(ops))
	default:
		t.Fatalf("unsupported border test command %q", command)
	}
	return append(args, attributes...)
}
