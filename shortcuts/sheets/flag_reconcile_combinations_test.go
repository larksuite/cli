// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestReconcile_FormulaRejectsConflictingInputs(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, param string
		args        []string
	}{
		{"constant cells and formula", "--formula", []string{"--cells", `[[{"value":1}]]`, "--formula", "=SUM(B1:B2)"}},
		{"different cell formula", "--formula", []string{"--cells", `[[{"formula":"=1"}]]`, "--formula", "=2"}},
		{"different formula aliases", "--formulas", []string{"--formula", "=1", "--formulas", "=2"}},
		{"formula does not name multiple cells", "--formula", []string{"--cells", `[[{"formula":"=1"},{"formula":"=1"}]]`, "--formula", "=1"}},
		{"writes and formula", "--formula", []string{"--writes", `[{"sheet_name":"S","range":"A1","cells":[[{"formula":"=1"}]]}]`, "--formula", "=1"}},
		{"empty formula", "--formula", []string{"--formula", ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), append([]string{
				"--url", testURL, "--sheet-name", "S", "--range", "A1", "--dry-run",
			}, tc.args...))
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Equal(t, tc.param, validation.Param)
			require.Empty(t, out)
		})
	}
}

func TestReconcile_FormulaAgreementPreservesCells(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{
		{"--formula", "=SUM(1,2)", "--formulas", "=SUM(1,2)"},
		{"--cells", `[[{"formula":"=SUM(1,2)","cell_styles":{"font_weight":"bold"}}]]`, "--formula", "=SUM(1,2)", "--formulas", "=SUM(1,2)"},
	} {
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), append([]string{
			"--url", testURL, "--sheet-name", "S", "--range", "A1",
		}, args...))
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		require.Equal(t, "=SUM(1,2)", gjson.Get(input, "cells.0.0.formula").String())
		if args[0] == "--cells" {
			require.Equal(t, "bold", gjson.Get(input, "cells.0.0.cell_styles.font_weight").String())
		}
	}
}

func TestReconcile_FormulaChecksResolvedCellsFile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	payload := filepath.Join(t.TempDir(), "cells.json")
	require.NoError(t, os.WriteFile(payload, []byte(`[[{"formula":"=1"}]]`), 0600))
	for _, formula := range []string{"=1", "=2"} {
		out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), []string{
			"--url", testURL, "--sheet-name", "S", "--range", "A1", "--cells", "@" + payload,
			"--formula", formula, "--dry-run",
		})
		if formula == "=1" {
			require.NoError(t, err)
			require.Contains(t, out, "formula")
		} else {
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, "--formula", validation.Param)
			require.Empty(t, out)
		}
	}
}

func TestReconcile_ExplicitBoldOutranksStyleEnvelope(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, command := range []string{"+cells-set-style", "+cells-batch-set-style"} {
		for _, tc := range []struct {
			name, envelope, weight string
			flags                  []string
		}{
			{"bold false", `{"font_weight":"bold"}`, "normal", []string{"--bold=false"}},
			{"font-bold false", `{"font_weight":"bold"}`, "normal", []string{"--font-bold=false"}},
			{"bold true", `{"font_weight":"normal"}`, "bold", []string{"--bold=true"}},
			{"font-bold outranks envelope alias", `{"bold":true}`, "normal", []string{"--font-bold=false"}},
			{"canonical flag retains precedence", `{"font_weight":"bold"}`, "normal", []string{"--font-weight", "normal", "--bold=true"}},
			{"envelope boolean still resolves", `{"bold":false}`, "normal", nil},
			{"envelope font-bold still resolves", `{"font_bold":false}`, "normal", nil},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				args := []string{"--url", testURL}
				path := "cells.0.0.cell_styles.font_weight"
				if command == "+cells-set-style" {
					args = append(args, "--sheet-name", "S", "--range", "A1")
				} else {
					args = append(args, "--ranges", `["S!A1"]`)
					path = "operations.0.input.cells.0.0.cell_styles.font_weight"
				}
				args = append(args, "--style", tc.envelope)
				calls := parseDryRunAPI(t, shortcutFromRegistry(t, command), append(args, tc.flags...))
				input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
				require.Equal(t, tc.weight, gjson.Get(input, path).String(), input)
			})
		}
	}
}

func TestReconcile_StyleEnvelopeSpellingsAgreeOrConflict(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, command := range []string{"+cells-set-style", "+cells-batch-set-style"} {
		t.Run(command+"/equal", func(t *testing.T) {
			args := []string{"--url", testURL, "--style", `{"font_weight":"bold"}`, "--styles", `{"font_weight":"bold"}`}
			path := "cells.0.0.cell_styles.font_weight"
			if command == "+cells-set-style" {
				args = append(args, "--sheet-name", "S", "--range", "A1")
			} else {
				args = append(args, "--ranges", `["S!A1"]`)
				path = "operations.0.input.cells.0.0.cell_styles.font_weight"
			}
			calls := parseDryRunAPI(t, shortcutFromRegistry(t, command), args)
			input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			require.Equal(t, "bold", gjson.Get(input, path).String(), input)
		})
		t.Run(command+"/different", func(t *testing.T) {
			args := []string{"--url", testURL, "--style", `{"font_weight":"bold"}`, "--styles", `{"font_weight":"normal"}`, "--dry-run"}
			if command == "+cells-set-style" {
				args = append(args, "--sheet-name", "S", "--range", "A1")
			} else {
				args = append(args, "--ranges", `["S!A1"]`)
			}
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, command), args)
			validation := requireValidation(t, err, "different envelopes")
			require.Equal(t, "--styles", validation.Param)
		})
	}
}
func TestReconcile_BoldAliasesRejectConflicts(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, command := range []string{"+cells-set-style", "+cells-batch-set-style"} {
		t.Run(command+"/explicit", func(t *testing.T) {
			args := []string{"--url", testURL, "--bold=false", "--font-bold=true", "--dry-run"}
			if command == "+cells-set-style" {
				args = append(args, "--sheet-name", "S", "--range", "A1")
			} else {
				args = append(args, "--ranges", `["S!A1"]`)
			}
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, command), args)
			validation := requireValidation(t, err, "conflicting bold values")
			require.Equal(t, "--bold", validation.Param)
		})
		t.Run(command+"/style-envelope", func(t *testing.T) {
			args := []string{"--url", testURL, "--style", `{"bold":false,"font_bold":true}`, "--dry-run"}
			if command == "+cells-set-style" {
				args = append(args, "--sheet-name", "S", "--range", "A1")
			} else {
				args = append(args, "--ranges", `["S!A1"]`)
			}
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, command), args)
			requireValidation(t, err, "conflicting bold values")
		})
	}
}
func TestReconcile_DeleteAliasCountBatchMatchesStandalone(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, key := range []string{"start-index", "start_index", "start", "position", "range"} {
		t.Run(key, func(t *testing.T) {
			op := map[string]interface{}{
				"shortcut": "+dim-delete",
				"input":    map[string]interface{}{"sheet_name": "S", key: 3, "count": 2},
			}
			operations, err := json.Marshal([]interface{}{op})
			require.NoError(t, err)
			batchCalls := parseDryRunAPI(t, shortcutFromRegistry(t, "+batch-update"), []string{
				"--url", testURL, "--operations", string(operations),
			})
			batchInput := batchCalls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			require.Equal(t, "3:4", gjson.Get(batchInput, "operations.0.input.range").String())
			standaloneCalls := parseDryRunAPI(t, shortcutFromRegistry(t, "+dim-delete"), []string{
				"--url", testURL, "--sheet-name", "S", "--" + key, "3", "--count", "2",
			})
			standaloneInput := standaloneCalls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			require.JSONEq(t, standaloneInput, gjson.Get(batchInput, "operations.0.input").Raw)
		})
	}
}
