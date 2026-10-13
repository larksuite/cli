// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
)

func TestReconcile_DimensionAliasRequests(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, command, position, span string
		args                          []string
		count                         int
	}{
		{"insert zero-based indexes", "+dim-insert", "4", "", []string{"--start-index", "3", "--end-index", "7"}, 4},
		{"insert at index zero", "+dim-insert", "1", "", []string{"--start-index", "0", "--end-index", "2"}, 2},
		{"insert index and count", "+dim-insert", "4", "", []string{"--start-index", "3", "--count", "2"}, 2},
		{"canonical position stays one-based", "+dim-insert", "3", "", []string{"--position", "3", "--count", "2"}, 2},
		{"insert agreeing count and end", "+dim-insert", "4", "", []string{"--start-index", "3", "--end-index", "7", "--count", "4"}, 4},
		{"insert singleton column span", "+dim-insert", "C", "", []string{"--range", "C:C", "--count", "1", "--dimension", "column"}, 1},
		{"delete inclusive indexes", "+dim-delete", "", "3:7", []string{"--start-index", "3", "--end-index", "7"}, 0},
		{"delete single row", "+dim-delete", "", "3:3", []string{"--start-index", "3", "--end-index", "3"}, 0},
		{"delete agreeing count and end", "+dim-delete", "", "6:22", []string{"--start-index", "6", "--end-index", "22", "--count", "17"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), append([]string{
				"--url", testURL, "--sheet-name", "S",
			}, tc.args...))
			require.Len(t, calls, 1)
			call := calls[0].(map[string]interface{})
			require.Equal(t, "POST", call["method"])
			body := call["body"].(map[string]interface{})
			require.Equal(t, "modify_sheet_structure", body["tool_name"])
			var input map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(body["input"].(string)), &input))
			if tc.position != "" {
				require.Equal(t, tc.position, input["position"])
				require.Equal(t, float64(tc.count), input["count"])
			} else {
				require.Equal(t, tc.span, input["range"])
			}
		})
	}
}

func TestReconcile_DimensionAliasRejectsUnsafeInputs(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, command, param string
		args                 []string
		wantCause            bool
	}{
		{"canonical row conflicts with dimension", "+dim-insert", "--dimension", []string{"--position", "3", "--count", "1", "--dimension", "column"}, false},
		{"canonical column conflicts with dimension", "+dim-insert", "--dimension", []string{"--position", "C", "--count", "1", "--dimension", "row"}, false},
		{"converted row conflicts with dimension", "+dim-insert", "--dimension", []string{"--start-index", "2", "--end-index", "4", "--dimension", "column"}, false},
		{"reversed range", "+dim-insert", "--range", []string{"--range", "6:3", "--count", "2"}, true},
		{"invalid range end", "+dim-insert", "--range", []string{"--range", "3:garbage", "--count", "1"}, true},
		{"non-singleton range", "+dim-insert", "--range", []string{"--range", "3:6", "--count", "1"}, false},
		{"range disagrees with position", "+dim-insert", "--range", []string{"--position", "C", "--range", "3:3", "--count", "1"}, false},
		{"count disagrees with end", "+dim-insert", "--end-index", []string{"--start-index", "3", "--end-index", "7", "--count", "1"}, false},
		{"invalid end with count", "+dim-insert", "--end-index", []string{"--position", "3", "--end-index", "bad", "--count", "2"}, true},
		{"invalid start index", "+dim-insert", "--start-index", []string{"--start-index", "bad", "--count", "1"}, true},
		{"negative start index", "+dim-insert", "--start-index", []string{"--start-index", "-1", "--count", "1"}, false},
		{"delete end before start", "+dim-delete", "--end-index", []string{"--start-index", "6", "--end-index", "5"}, false},
		{"delete count disagrees with end", "+dim-delete", "--range", []string{"--start-index", "6", "--end-index", "22", "--count", "16"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, tc.command), append([]string{
				"--url", testURL, "--sheet-name", "S", "--dry-run",
			}, tc.args...))
			var ve *errs.ValidationError
			require.ErrorAs(t, err, &ve)
			require.Equal(t, errs.SubtypeInvalidArgument, ve.Subtype)
			require.Equal(t, tc.param, ve.Param)
			require.Empty(t, out, "invalid input must not produce an API plan")
			if tc.wantCause {
				require.NotNil(t, errors.Unwrap(ve))
			}
		})
	}
}

func TestReconcile_QualifiedDeleteIndexesCheckDimension(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct{ start, sheet, span, dimension, conflicting string }{
		{"S!3", "S", "3:5", "row", "column"},
		{"'Sales 1'!C", "Sales 1", "C:E", "column", "row"},
		{"工作表！C", "工作表", "C:E", "column", "row"},
	} {
		t.Run(tc.start, func(t *testing.T) {
			args := []string{"--url", testURL, "--start-index", tc.start, "--end-index", "5", "--dimension", tc.conflicting, "--dry-run"}
			out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+dim-delete"), args)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Equal(t, "--dimension", validation.Param)
			require.Empty(t, out, "a conflicting axis must not produce an API plan")

			args[len(args)-2] = tc.dimension
			calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+dim-delete"), args)
			require.Len(t, calls, 1)
			body := calls[0].(map[string]interface{})["body"].(map[string]interface{})
			var input map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(body["input"].(string)), &input))
			require.Equal(t, tc.sheet, input["sheet_name"])
			require.Equal(t, tc.span, input["range"])
		})
	}
}

func TestReconcile_BoldFalsePreservesStyle(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{
		{"--bold=false"}, {"--font-bold=false"}, {"--style", `{"bold":false}`},
	} {
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set-style"), append([]string{
			"--url", testURL, "--sheet-name", "S", "--range", "A1",
		}, args...))
		body := calls[0].(map[string]interface{})["body"].(map[string]interface{})
		var input struct {
			Cells [][]struct {
				Styles struct {
					Weight string `json:"font_weight"`
				} `json:"cell_styles"`
			} `json:"cells"`
		}
		require.NoError(t, json.Unmarshal([]byte(body["input"].(string)), &input))
		require.Equal(t, "normal", input.Cells[0][0].Styles.Weight)
	}
}

func TestReconcile_InsertIndexBatchMatchesStandalone(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, index := range []int{0, 3} {
		for _, key := range []string{"start-index", "start_index"} {
			input := map[string]interface{}{"sheet_name": "S", key: index, "count": 2}
			require.NoError(t, normalizeSubOpInputKeys("+dim-insert", input))
			fv := newMapFlagViewForCommand("+dim-insert", input)
			body, err := dimInsertInput(fv, testToken, "", "S")
			require.NoError(t, err)
			require.Equal(t, strconv.Itoa(index+1), body["position"])
			require.Equal(t, 2, body["count"])
			calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+dim-insert"), []string{
				"--url", testURL, "--sheet-name", "S", "--start-index", strconv.Itoa(index), "--count", "2",
			})
			standalone := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			batch, err := json.Marshal(body)
			require.NoError(t, err)
			require.JSONEq(t, standalone, string(batch))
		}
	}
}
