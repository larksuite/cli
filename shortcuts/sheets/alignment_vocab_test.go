// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAlignmentVocabularyCarrierParity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, style := range []string{
		`{"alignment":{"horizontal":"right","vertical":"bottom","wrapText":false},"font_weight":"bold"}`,
		`{"h_align":2,"v_align":2,"word_wrap":false,"font_weight":"bold"}`,
		`{"horizontal_alignment":2,"vertical_alignment":2,"word_wrap":false,"font_weight":"bold"}`,
	} {
		for _, tc := range []struct {
			name, command, path string
			args                []string
		}{
			{"style envelope", "+cells-set-style", "cells.0.0", []string{"--sheet-name", "S", "--range", "A2", "--style", style}},
			{"batch style envelope", "+cells-batch-set-style", "operations.0.input.cells.0.0", []string{"--ranges", `["S!A2"]`, "--styles", style}},
			{"typed cells", "+cells-set", "cells.0.0", []string{"--sheet-name", "S", "--range", "A2", "--cells", `[[{"value":"keep","cell_styles":` + style + `}]]`}},
			{"writes", "+cells-set", "operations.0.input.cells.0.0", []string{"--writes", `[{"sheet_name":"S","range":"A2","cells":[[{"value":"keep","cell_styles":` + style + `}]]}]`}},
			{"sparse cells", "+cells-set", "operations.0.input.cells.0.0", []string{"--sheet-name", "S", "--cells", `[{"cell":"A2","value":"keep","cell_styles":` + style + `}]`}},
			{"declarative styles", "+styles-put", "operations.0.input.cells.0.0", []string{"--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"A2",` + style[1:] + `]}]}`}},
			{"batch sub-operation", "+batch-update", "operations.0.input.cells.0.0", []string{"--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A2","style":` + style + `}}]`}},
		} {
			t.Run(style+"/"+tc.name, func(t *testing.T) {
				api := parseDryRunAPI(t, shortcutFromRegistry(t, tc.command), append([]string{"--url", testURL}, tc.args...))
				input := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
				cell := gjson.Get(input, tc.path)
				require.Equal(t, "right", cell.Get("cell_styles.horizontal_alignment").String(), input)
				require.Equal(t, "bottom", cell.Get("cell_styles.vertical_alignment").String(), input)
				require.Equal(t, "overflow", cell.Get("cell_styles.word_wrap").String(), input)
				require.Equal(t, "bold", cell.Get("cell_styles.font_weight").String(), input)
				require.False(t, cell.Get("cell_styles.alignment").Exists(), input)
				if tc.command == "+cells-set" {
					require.Equal(t, "keep", cell.Get("value").String(), input)
				} else {
					require.False(t, cell.Get("value").Exists(), input)
				}
			})
		}
	}
}

func TestAlignmentVocabularyUsesLarkNumericEnum(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		value                int
		horizontal, vertical string
	}{{0, "left", "top"}, {1, "center", "middle"}, {2, "right", "bottom"}} {
		api := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
			"--url", testURL, "--sheet-name", "S", "--range", "A2", "--style",
			fmt.Sprintf(`{"hAlign":%d,"vAlign":%d}`, tc.value, tc.value),
		})
		input := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		require.Equal(t, tc.horizontal, gjson.Get(input, "cells.0.0.cell_styles.horizontal_alignment").String())
		require.Equal(t, tc.vertical, gjson.Get(input, "cells.0.0.cell_styles.vertical_alignment").String())
	}
}

func TestAlignmentVocabularyRejectsUnsupportedFieldsAndConflicts(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, style := range []string{
		`{"alignment":{}}`,
		`{"alignment":"center"}`,
		`{"alignment":null}`,
		`{"alignment":{"horizontal":null}}`,
		`{"alignment":{"horizontal":false}}`,
		`{"alignment":{"horizontal":"general"}}`,
		`{"alignment":{"horizontal":"center","indent":1}}`,
		`{"alignment":{"wrapText":true,"shrinkToFit":true}}`,
		`{"alignment":{"textRotation":90}}`,
		`{"alignment":{"horizontal":"center","h_align":"left"}}`,
		`{"alignment":{"horizontal":"center"},"horizontal_alignment":"left"}`,
		`{"alignment":{"vertical":"top"},"v_align":"bottom"}`,
		`{"alignment":{"wrapText":false},"word_wrap":"auto-wrap"}`,
		`{"horizontal_alignment":3}`,
		`{"vertical_alignment":-1}`,
		`{"horizontal_alignment":1.5}`,
		`{"h_align":"1"}`,
	} {
		t.Run(style, func(t *testing.T) {
			for _, command := range []string{"+cells-set-style", "+styles-put", "+cells-set"} {
				if command == "+cells-set" && (style == `{"alignment":"center"}` || style == `{"alignment":null}`) {
					continue // pinned separately: loose cell_styles preserves these fields
				}
				args := []string{"--url", testURL, "--dry-run"}
				param := "--style"
				switch command {
				case "+cells-set-style":
					args = append(args, "--sheet-name", "S", "--range", "A2", "--style", style)
				case "+styles-put":
					param = "--styles"
					args = append(args, "--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"A2",`+style[1:]+`]}]}`)
				case "+cells-set":
					param = "--cells"
					args = append(args, "--sheet-name", "S", "--range", "A2", "--cells", `[[{"value":"keep","cell_styles":`+style+`}]]`)
				}
				out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, command), args)
				var validation *errs.ValidationError
				require.ErrorAs(t, err, &validation)
				require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
				require.Equal(t, param, validation.Param)
				require.Empty(t, out)
			}
		})
	}
}

func TestAlignmentVocabularyPreservesLooseTypedCellFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, value := range []string{`"center"`, `null`, `false`, `7`} {
		for _, sparse := range []bool{false, true} {
			args := []string{"--url", testURL, "--sheet-name", "S"}
			cell := `{"value":"keep","style":{"bold":true,"alignment":` + value + `}}`
			path := "cells.0.0"
			if sparse {
				args = append(args, "--cells", `[`+strings.Replace(cell, `{"value"`, `{"cell":"A2","value"`, 1)+`]`)
				path = "operations.0.input.cells.0.0"
			} else {
				args = append(args, "--range", "A2", "--cells", `[[`+cell+`]]`)
			}
			api := parseDryRunAPI(t, shortcutFromRegistry(t, "+cells-set"), args)
			input := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
			require.Equal(t, value, gjson.Get(input, path+".cell_styles.alignment").Raw, input)
			require.Equal(t, "bold", gjson.Get(input, path+".cell_styles.font_weight").String(), input)
			require.Equal(t, "keep", gjson.Get(input, path+".value").String(), input)
			require.False(t, gjson.Get(input, path+".cell_styles.horizontal_alignment").Exists(), input)
		}
	}
}

func TestNestedAlignmentExplicitFlagsKeepPrecedence(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, command := range []string{"+cells-set-style", "+batch-update"} {
		args := []string{"--url", testURL}
		path := "cells.0.0"
		style := `{"alignment":{"horizontal":"invalid-shadowed","vertical":"center","wrapText":{}}}`
		if command == "+batch-update" {
			args = append(args, "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":"S","range":"A2","horizontal_alignment":"right","word_wrap":"word-clip","style":`+style+`}}]`)
			path = "operations.0.input.cells.0.0"
		} else {
			args = append(args, "--sheet-name", "S", "--range", "A2", "--horizontal-alignment", "right", "--word-wrap", "word-clip", "--style", style)
		}
		api := parseDryRunAPI(t, shortcutFromRegistry(t, command), args)
		input := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
		cell := gjson.Get(input, path)
		require.Equal(t, "right", cell.Get("cell_styles.horizontal_alignment").String(), input)
		require.Equal(t, "middle", cell.Get("cell_styles.vertical_alignment").String(), input)
		require.Equal(t, "word-clip", cell.Get("cell_styles.word_wrap").String(), input)
	}
	// An explicit horizontal flag cannot hide a different, unsupported option.
	out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set-style"), []string{
		"--url", testURL, "--sheet-name", "S", "--range", "A2", "--horizontal-alignment", "right",
		"--style", `{"alignment":{"horizontal":"center","indent":1}}`, "--dry-run",
	})
	var validation *errs.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, "--style", validation.Param)
	require.Empty(t, out)
}

func TestAlignmentEnvelopeCasingKeepsExplicitFlagPrecedence(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, container := range []string{"alignment", "Alignment", "ALIGNMENT"} {
		for _, invalid := range []string{`null`, `"invalid"`} {
			for _, command := range []string{"+cells-set-style", "+batch-update"} {
				for _, override := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/override=%t", container, invalid, command, override), func(t *testing.T) {
						style := fmt.Sprintf(`{%q:{"horizontal":%s,"vertical":"top"}}`, container, invalid)
						args := []string{"--url", testURL}
						path, param := "cells.0.0", "--style"
						if command == "+batch-update" {
							explicit := ""
							if override {
								explicit = `,"horizontal_alignment":"center"`
							}
							args = append(args, "--yes", "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_id":"`+testSheetID+`","range":"A1","style":`+style+explicit+`}}]`)
							path, param = "operations.0.input.cells.0.0", "--operations"
						} else {
							args = append(args, "--sheet-id", testSheetID, "--range", "A1", "--style", style)
							if override {
								args = append(args, "--horizontal-alignment", "center")
							}
						}
						write := toolOutputStub(testToken, "write", `{}`)
						write.Optional = !override
						out, err := runShortcutWithStubs(t, shortcutFromRegistry(t, command), args, write)
						if !override {
							var validation *errs.ValidationError
							require.ErrorAs(t, err, &validation)
							require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
							require.Equal(t, param, validation.Param)
							require.Empty(t, write.CapturedBodies)
							require.Empty(t, out)
							return
						}
						require.NoError(t, err)
						require.Len(t, write.CapturedBodies, 1)
						api := parseDryRunAPI(t, shortcutFromRegistry(t, command), args)
						preview := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
						for _, input := range []string{preview, gjson.GetBytes(write.CapturedBody, "input").String()} {
							cell := gjson.Get(input, path)
							require.Equal(t, "center", cell.Get("cell_styles.horizontal_alignment").String())
							require.Equal(t, "top", cell.Get("cell_styles.vertical_alignment").String())
							require.False(t, cell.Get("value").Exists())
						}
					})
				}
			}
		}
	}
}

func TestNumericAlignmentAcrossCellCarriers(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct{ name, path, field, carrier string }{
		{"dense horizontal", "cells.0.0", "hAlign", "dense"},
		{"dense vertical", "cells.0.0", "vAlign", "dense"},
		{"writes horizontal", "operations.0.input.cells.0.0", "horizontal", "writes"},
		{"writes vertical", "operations.0.input.cells.0.0", "vertical", "writes"},
		{"sparse horizontal", "operations.0.input.cells.0.0", "horizontal_alignment", "sparse"},
		{"sparse vertical", "operations.0.input.cells.0.0", "vertical_alignment", "sparse"},
		{"style envelope horizontal", "cells.0.0", "hAlign", "style"},
		{"style envelope vertical", "cells.0.0", "vAlign", "style"},
	} {
		for _, number := range []struct{ raw, horizontal, vertical string }{
			{"0.0", "left", "top"}, {"1e0", "center", "middle"}, {"2.0", "right", "bottom"},
			{"1.0000000000000000001", "", ""}, {"2.0000000000000000001", "", ""}, {"-1e-400", "", ""},
		} {
			t.Run(tc.name+"/"+number.raw, func(t *testing.T) {
				style := fmt.Sprintf(`{%q:%s}`, tc.field, number.raw)
				if tc.carrier == "writes" {
					style = `{"alignment":` + style + `}`
				}
				cell := `{"value":9007199254740993,"cell_styles":` + style + `}`
				args := []string{"--url", testURL, "--sheet-id", testSheetID}
				param := "--cells"
				shortcut := CellsSet
				switch tc.carrier {
				case "dense":
					args = append(args, "--range", "A1", "--cells", `[[`+cell+`]]`)
				case "writes":
					args = append(args, "--writes", `[{"range":"A1","cells":[[`+cell+`]]}]`)
					param = "--writes"
				case "sparse":
					args = append(args, "--cells", `[{"cell":"A1",`+cell[1:]+`]`)
				case "style":
					args = append(args, "--range", "A1", "--style", style)
					param, shortcut = "--style", shortcutFromRegistry(t, "+cells-set-style")
				}
				valid := number.horizontal != ""
				write := toolOutputStub(testToken, "write", `{}`)
				write.Optional = !valid
				out, err := runShortcutWithStubs(t, shortcut, args, write)
				if !valid {
					var validation *errs.ValidationError
					require.ErrorAs(t, err, &validation)
					require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
					require.Equal(t, param, validation.Param)
					require.Empty(t, write.CapturedBodies)
					require.Empty(t, out)
					return
				}
				require.NoError(t, err)
				require.Len(t, write.CapturedBodies, 1)
				api := parseDryRunAPI(t, shortcut, args)
				preview := api[0].(map[string]interface{})["body"].(map[string]interface{})["input"].(string)
				field, want := "horizontal_alignment", number.horizontal
				if strings.Contains(tc.name, "vertical") {
					field, want = "vertical_alignment", number.vertical
				}
				for _, input := range []string{preview, gjson.GetBytes(write.CapturedBody, "input").String()} {
					leaf := gjson.Get(input, tc.path)
					require.Equal(t, want, leaf.Get("cell_styles."+field).String())
					if tc.carrier == "style" {
						require.False(t, leaf.Get("value").Exists())
					} else {
						require.Equal(t, "9007199254740993", leaf.Get("value").Raw)
					}
				}
			})
		}
	}
}
