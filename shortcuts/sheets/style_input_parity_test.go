// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The October 7–8 corpus includes the same style vocabulary in envelopes,
// typed cells and declarative specs. All must produce the same cell stamp.
func TestStyleInputParity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	style := `{"font":{"bold":false,"italic":true,"fontSize":"14","name":"Arial","color":"#123456"},"h_align":"center","v_align":"center","wrap_text":true,"border":{"bottom":{"style":"dashed","weight":"thin"}}}`
	for _, tc := range []struct {
		command, path string
		args          []string
	}{
		{"+cells-set-style", "cells.0.0", []string{"--sheet-name", "S", "--range", "A1", "--style", style}},
		{"+cells-batch-set-style", "operations.0.input.cells.0.0", []string{"--ranges", `["S!A1"]`, "--styles", style}},
		{"+cells-set", "cells.0.0", []string{"--sheet-name", "S", "--range", "A1", "--cells", `[[{"value":"keep","cell_styles":` + style + `}]]`}},
		{"+styles-put", "operations.0.input.cells.0.0", []string{"--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"A1","font":{"bold":false,"italic":true,"fontSize":"14","name":"Arial","color":"#123456"},"h_align":"center","v_align":"center","wrap_text":true,"border":{"bottom":{"style":"dashed","weight":"thin"}}}]}]}`}},
		{"+batch-update", "operations.0.input.cells.0.0", []string{"--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A1","style":` + style + `}}]`}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			calls := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), append([]string{"--url", testURL}, tc.args...))
			input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			cell := gjson.Get(input, tc.path)
			for field, want := range map[string]string{
				"font_weight": "normal", "font_style": "italic", "font_family": "Arial", "font_color": "#123456",
				"horizontal_alignment": "center", "vertical_alignment": "middle", "word_wrap": "auto-wrap",
			} {
				require.Equal(t, want, cell.Get("cell_styles."+field).String(), input)
			}
			require.Equal(t, float64(14), cell.Get("cell_styles.font_size").Float(), input)
			require.Equal(t, "dashed", cell.Get("border_styles.bottom.style").String(), input)
			require.Equal(t, "thin", cell.Get("border_styles.bottom.weight").String(), input)
			require.False(t, cell.Get("cell_styles.font").Exists(), input)
			if tc.command == "+cells-set" {
				require.Equal(t, "keep", cell.Get("value").String(), input)
			} else {
				require.False(t, cell.Get("value").Exists(), input)
			}
		})
	}
}

func TestStyleEnvelopeRejectsUnsupportedOrConflictingFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, style := range []string{
		`{"font":{"bold":true,"shadow":true}}`,
		`{"font":"Arial"}`,
		`{"font":{}}`,
		`{"font":{"bold":true},"font_weight":"normal"}`,
		`{"font":{"size":12,"font_size":14}}`,
		`{"font":{"bold":true,"weight":"normal"}}`,
		`{"font":{"bold":"perhaps"}}`,
		`{"font":{"size":"large"}}`,
		`{"font":{"color":{"rgb":"FFFFFF"}}}`,
		`{"h_align":"center","horizontal_alignment":"left"}`,
		`{"range":"B1","font_weight":"bold"}`,
		`{"value":"overwrite","font_weight":"bold"}`,
		`{"border_styles":{"bottom":{"style":"invalid"}}}`,
	} {
		t.Run(style, func(t *testing.T) {
			out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
				"--url", testURL, "--sheet-name", "S", "--range", "A1", "--style", style, "--dry-run",
			})
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Equal(t, "--style", validation.Param)
			require.Empty(t, out)
		})
	}
}

func TestStyleEnvelopeKeepsCanonicalSpellingsAndBoldAgreement(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, style := range []string{
		`{"bold":false,"font_bold":false,"fontSize":14}`,
		`{"font-weight":"normal","bold":true,"font_bold":false,"font-size":14}`,
		`{"font_weight":"normal","bold":true,"font_size":14}`,
		`{"Bold":false,"FontBold":false,"FontSize":14}`,
	} {
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
			"--url", testURL, "--sheet-name", "S", "--range", "A1", "--style", style,
		})
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		require.Equal(t, "normal", gjson.Get(input, "cells.0.0.cell_styles.font_weight").String(), input)
		require.Equal(t, int64(14), gjson.Get(input, "cells.0.0.cell_styles.font_size").Int(), input)
	}
}

func TestStyleEnvelopeKeepsFlagVocabularyAliases(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
		"--url", testURL, "--sheet-name", "S", "--range", "A1", "--style",
		`{"range":"A1","backColor":"#EEEEEE","textColor":"#123456","hAlign":"center","vAlign":"center","wrapText":true,"borderType":"FULL_BORDER"}`,
	})
	input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
	for field, want := range map[string]string{
		"background_color": "#EEEEEE", "font_color": "#123456", "horizontal_alignment": "center",
		"vertical_alignment": "middle", "word_wrap": "auto-wrap",
	} {
		require.Equal(t, want, gjson.Get(input, "cells.0.0.cell_styles."+field).String(), input)
	}
	require.Equal(t, "A1", gjson.Get(input, "range").String(), input)
	require.Equal(t, "solid", gjson.Get(input, "cells.0.0.border_styles.bottom.style").String(), input)
}

func TestStyleEnvelopeKeepsStringBordersAndRange(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, style := range []string{
		`{"range":"A1","border_styles":"FULL_BORDER"}`,
		`{"range":"A1","border_styles":"{\"all\":{\"style\":\"solid\"}}"}`,
	} {
		calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
			"--url", testURL, "--sheet-name", "S", "--style", style,
		})
		input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		require.Equal(t, "A1", gjson.Get(input, "range").String(), input)
		require.Equal(t, "solid", gjson.Get(input, "cells.0.0.border_styles.bottom.style").String(), input)
	}
}

func TestStyleEnvelopeExplicitFlagsKeepPrecedence(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, command := range []string{"+cells-set-style", "+batch-update"} {
		t.Run(command, func(t *testing.T) {
			args := []string{"--url", testURL}
			path := "cells.0.0"
			if command == "+batch-update" {
				args = append(args, "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A1","bold":false,"font_size":18,"style":{"font":{"bold":true,"size":12},"border":{"all":{"style":"solid"}}},"border_styles":{"bottom":{"style":"dotted"}}}}]`)
				path = "operations.0.input.cells.0.0"
			} else {
				args = append(args, "--sheet-name", "S", "--range", "A1", "--bold=false", "--font-size", "18",
					"--style", `{"font":{"bold":true,"size":12},"border":{"all":{"style":"solid"}}}`,
					"--border-styles", `{"bottom":{"style":"dotted"}}`)
			}
			calls := parseDryRunAPI(t, shortcutFromRegistry(t, command), args)
			input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			cell := gjson.Get(input, path)
			require.Equal(t, "normal", cell.Get("cell_styles.font_weight").String(), input)
			require.Equal(t, int64(18), cell.Get("cell_styles.font_size").Int(), input)
			require.Equal(t, "dotted", cell.Get("border_styles.bottom.style").String(), input)
			require.False(t, cell.Get("border_styles.top").Exists(), input)
		})
	}
}

func TestStyleEnvelopePreciseNumbersKeepExplicitPrecedence(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, number := range []string{"1.0000000000000000001", "2.0000000000000000001", "-1e-400"} {
		t.Run(number, func(t *testing.T) {
			args := []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1", "--horizontal-alignment", "center", "--style",
				`{"hAlign":` + number + `,"vAlign":0.0,"font":{"size":14,"italic":1},"border":{"bottom":{"style":"solid","weight":2}}}`}
			sc := shortcutFromRegistry(t, "+cells-set-style")
			write := toolOutputStub(testToken, "write", `{}`)
			_, err := runShortcutWithStubs(t, sc, args, write)
			require.NoError(t, err)
			require.Len(t, write.CapturedBodies, 1)
			api := parseDryRunAPI(t, sc, args)
			preview := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			for _, input := range []string{preview, gjson.GetBytes(write.CapturedBody, "input").String()} {
				leaf := gjson.Get(input, "cells.0.0")
				require.Equal(t, "center", leaf.Get("cell_styles.horizontal_alignment").String())
				require.Equal(t, "top", leaf.Get("cell_styles.vertical_alignment").String())
				require.Equal(t, int64(14), leaf.Get("cell_styles.font_size").Int())
				require.Equal(t, "italic", leaf.Get("cell_styles.font_style").String())
				require.Equal(t, "medium", leaf.Get("border_styles.bottom.weight").String())
				require.False(t, leaf.Get("value").Exists())
			}
		})
	}
}

func TestStyleEnvelopeDecodePreservesShapeAndSyntaxErrorCauses(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct{ payload, cause string }{
		{`[]`, "type"}, {`"center"`, "type"}, {`14`, "type"},
		{`{"hAlign":`, "syntax"}, {`{"hAlign":0} trailing`, "syntax"},
		{`null`, ""}, {`{}`, ""},
	} {
		t.Run(tc.payload, func(t *testing.T) {
			write := toolOutputStub(testToken, "write", `{}`)
			write.Optional = true
			out, err := runShortcutWithStubs(t, shortcutFromRegistry(t, "+cells-set-style"), []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1", "--style", tc.payload}, write)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Equal(t, "--style", validation.Param)
			switch tc.cause {
			case "type":
				var cause *json.UnmarshalTypeError
				require.ErrorAs(t, validation.Unwrap(), &cause)
			case "syntax":
				var cause *json.SyntaxError
				require.ErrorAs(t, validation.Unwrap(), &cause)
				require.Positive(t, cause.Offset)
			case "":
				require.Nil(t, validation.Unwrap())
			}
			require.Empty(t, write.CapturedBodies)
			require.Empty(t, out)
		})
	}
}

func TestStyleEnvelopeBorderDuplicateNumbersUseExactEquality(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, outer := range []string{"1.0", "1e0", "2", "1.0000000000000000001"} {
		t.Run(outer, func(t *testing.T) {
			args := []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1", "--style",
				`{"border_styles":{"all":{"style":"solid","weight":1},"outer":{"style":"solid","weight":` + outer + `}}}`}
			sc := shortcutFromRegistry(t, "+cells-set-style")
			valid := outer == "1.0" || outer == "1e0"
			write := toolOutputStub(testToken, "write", `{}`)
			write.Optional = !valid
			out, err := runShortcutWithStubs(t, sc, args, write)
			if !valid {
				var validation *errs.ValidationError
				require.ErrorAs(t, err, &validation)
				require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
				require.Equal(t, "--style", validation.Param)
				require.Empty(t, write.CapturedBodies)
				require.Empty(t, out)
				return
			}
			require.NoError(t, err)
			require.Len(t, write.CapturedBodies, 1)
			api := parseDryRunAPI(t, sc, args)
			preview := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			for _, input := range []string{preview, gjson.GetBytes(write.CapturedBody, "input").String()} {
				border := gjson.Get(input, "cells.0.0.border_styles")
				for _, side := range []string{"top", "bottom", "left", "right"} {
					require.Equal(t, "solid", border.Get(side+".style").String())
					require.Equal(t, "thin", border.Get(side+".weight").String())
				}
				require.False(t, border.Get("all").Exists())
				require.False(t, border.Get("outer").Exists())
			}
		})
	}
}

func TestStyleEnvelopeScalarBorderStyleMatchesBatch(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, command := range []string{"+cells-set-style", "+batch-update"} {
		t.Run(command, func(t *testing.T) {
			args := []string{"--url", testURL}
			path := "cells.0.0"
			if command == "+batch-update" {
				args = append(args, "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A1","style":{"border_style":"thin"}}}]`)
				path = "operations.0.input.cells.0.0"
			} else {
				args = append(args, "--sheet-name", "S", "--range", "A1", "--style", `{"border_style":"thin"}`)
			}
			calls := parseDryRunAPI(t, shortcutFromRegistry(t, command), args)
			input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			cell := gjson.Get(input, path)
			require.Equal(t, "solid", cell.Get("border_styles.bottom.style").String(), input)
			require.Equal(t, "thin", cell.Get("border_styles.bottom.weight").String(), input)
		})
	}
}

func TestStyleEnvelopeExplicitFlagsMaskInvalidShadowedValues(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, flag, value, envelope, path, want string
	}{
		{"flat font weight", "font-weight", "bold", `{"font_weight":"invalid","font_size":14}`, "cell_styles.font_weight", "bold"},
		{"nested font weight", "font-weight", "bold", `{"font":{"weight":"invalid","size":14}}`, "cell_styles.font_weight", "bold"},
		{"background alias", "background-color", "#FFFFFF", `{"bg_color":123,"font_size":14}`, "cell_styles.background_color", "#FFFFFF"},
		{"fill alias", "background-color", "#FFFFFF", `{"fill_color":123,"font_size":14}`, "cell_styles.background_color", "#FFFFFF"},
		{"border side alias", "border-styles", `{"bottom":{"style":"solid"}}`, `{"border_top":{"style":"invalid"},"font_size":14}`, "border_styles.bottom.style", "solid"},
		{"border family alias", "border-styles", `{"bottom":{"style":"solid"}}`, `{"borders":{"top":{"style":"invalid"}},"font_size":14}`, "border_styles.bottom.style", "solid"},
	} {
		for _, command := range []string{"+cells-set-style", "+batch-update"} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				args := []string{"--url", testURL}
				path := "cells.0.0"
				if command == "+batch-update" {
					flagValue, err := json.Marshal(tc.value)
					require.NoError(t, err)
					args = append(args, "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A1",`+strings.ReplaceAll(tc.flag, "-", "_")+`:`+string(flagValue)+`,"style":`+tc.envelope+`}}]`)
					path = "operations.0.input.cells.0.0"
				} else {
					args = append(args, "--sheet-name", "S", "--range", "A1", "--"+tc.flag, tc.value, "--style", tc.envelope)
				}
				calls := parseDryRunAPI(t, shortcutFromRegistry(t, command), args)
				input := calls[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
				cell := gjson.Get(input, path)
				require.Equal(t, tc.want, cell.Get(tc.path).String(), input)
				require.Equal(t, float64(14), cell.Get("cell_styles.font_size").Float(), input)
			})
		}
	}

	for _, command := range []string{"+cells-set-style", "+batch-update"} {
		t.Run(command+"/effective-invalid-value-remains-rejected", func(t *testing.T) {
			args := []string{"--url", testURL, "--dry-run"}
			wantParam := "--style"
			if command == "+batch-update" {
				args = append(args, "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A1","font_weight":"bold","style":{"font_weight":"invalid","font_style":"invalid"}}}]`)
				wantParam = "--operations"
			} else {
				args = append(args, "--sheet-name", "S", "--range", "A1", "--font-weight", "bold", "--style", `{"font_weight":"invalid","font_style":"invalid"}`)
			}
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, command), args)
			validation := requireValidation(t, err, "font_style value \"invalid\" is invalid")
			require.Equal(t, wantParam, validation.Param)
		})
	}
}

func TestStyleEnvelopeBorderShadowingDoesNotHideUnknownFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, command := range []string{"+cells-set-style", "+batch-update"} {
		t.Run(command, func(t *testing.T) {
			args := []string{"--url", testURL, "--dry-run"}
			wantParam := "--style"
			if command == "+batch-update" {
				args = append(args, "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A1","border_styles":{"bottom":{"style":"solid"}},"style":{"unexpected_border_mode":"invalid"}}}]`)
				wantParam = "--operations"
			} else {
				args = append(args, "--sheet-name", "S", "--range", "A1", "--border-styles", `{"bottom":{"style":"solid"}}`, "--style", `{"unexpected_border_mode":"invalid"}`)
			}
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, command), args)
			validation := requireValidation(t, err, "unexpected_border_mode is not a supported style field")
			require.Equal(t, wantParam, validation.Param)
		})
	}
}

func TestTypedCellsRejectBorderAliasWithMalformedBorderStyles(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, alias := range []string{
		`"border":{"bottom":{"style":"solid"}}`,
		`"border_type":"LEFT_BORDER"`,
	} {
		t.Run(alias, func(t *testing.T) {
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), []string{
				"--url", testURL, "--sheet-name", "S", "--range", "A1",
				"--cells", `[[{"cell_styles":{"border_styles":"dotted",` + alias + `}}]]`, "--dry-run",
			})
			validation := requireValidation(t, err, "border_styles must be an object")
			require.Equal(t, "--cells", validation.Param)
		})
	}
}
