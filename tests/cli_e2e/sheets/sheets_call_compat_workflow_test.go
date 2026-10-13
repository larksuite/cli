// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_CallCompatWorkflow drives the accepted input shapes against a real
// spreadsheet. The dry-run E2E pins what the CLI builds; this pins that the
// backend takes it — the rewrites turn caller spellings into wire payloads, and
// a payload the server rejects would be a worse outcome than the client-side
// error it replaced.
//
// The sheet is created with a space in its name on purpose: that forces the
// quoted prefix form ('cli e2e …'!A1), which is the spelling the ref-lexer
// grammar exists for and the one a first-ASCII-"!" split would cut in half.
//
// Covered in one disposable workbook, torn down by createSpreadsheet's cleanup:
//   - a sheet-qualified --range as the only selector (no --sheet-id/--sheet-name)
//   - bare scalars in cell slots, lifted into {"value": …}
//   - a bare single-cell --range as an anchor, sized from the payload
//   - the same prefix on the read side
//   - openpyxl's "hair" border weight, normalized to a contract weight
//   - explicit boolean bold aliases outrank the style envelope in both commands
//   - agreeing formula inputs write a formula that evaluates correctly
//   - nested font and border vocabulary survives each style input carrier
//   - coordinate-bearing sparse cells preserve the gap between their targets
//   - size lists and agreeing size-map ranges resize the exact rows/columns
//   - copy aliases retain the source, destination, and paste mode
//   - matching content type annotations preserve values and explicit formulas
//   - nested and Lark numeric alignment keep their exact style semantics
func TestSheets_CallCompatWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-sheets-callcompat-"+suffix, "bot")

	sheetName := "cli e2e " + suffix
	var explicitSheetID string
	qualified := func(rng string) string { return "'" + sheetName + "'!" + rng }

	t.Run("create the target sheet", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+sheet-create",
				"--spreadsheet-token", spreadsheetToken,
				"--title", sheetName,
				"--index", "1",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		explicitSheetID = gjson.Get(result.Stdout, "data.sheet_id").String()
		require.NotEmpty(t, gjson.Get(result.Stdout, "data.sheet_id").String(),
			"stdout:\n%s", result.Stdout)
	})

	// Three rewrites at once, and no selector flag anywhere: the prefix has to
	// fill sheet_name, the scalars have to lift, and the bare A1 has to size
	// itself to A1:B2 — the prefix is consumed before the anchor is read, so a
	// qualified range still expands here (unlike one sitting beside a selector
	// it contradicts, which stays a local mismatch by design).
	t.Run("write through a qualified anchor with scalar cells", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-set",
				"--spreadsheet-token", spreadsheetToken,
				"--range", qualified("A1"),
				"--cells", `[["名称","数量"],["电动大门",10331.5]]`,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("read it back through the same prefix", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-get",
				"--spreadsheet-token", spreadsheetToken,
				"--range", qualified("A1:B2"),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Scalars are collected out of the decoded payload rather than matched
		// against a fixed path: get_cell_ranges' response nesting is the
		// backend's to change and is pinned nowhere in this repo, while the
		// values having survived the round trip is the actual claim.
		got := scalarsIn(gjson.Get(result.Stdout, "data"))
		for _, want := range []string{"名称", "数量", "电动大门"} {
			require.Contains(t, got, want, "read-back lost %q; stdout:\n%s", want, result.Stdout)
		}
		// The lifted number is compared numerically: whether it comes back as
		// 10331.5, "10331.50" or a display-formatted string is the sheet's
		// formatting to decide, not something this test should pin.
		var sawNumber bool
		for _, s := range got {
			if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && f == 10331.5 {
				sawNumber = true
				break
			}
		}
		require.True(t, sawNumber, "read-back lost the lifted number; stdout:\n%s", result.Stdout)
	})

	// hair is openpyxl's thinnest border and not in the contract enum; it
	// normalizes to thin, which the backend has to accept for the fallback to
	// be worth anything.
	t.Run("style it with an openpyxl border weight", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-set-style",
				"--spreadsheet-token", spreadsheetToken,
				"--range", qualified("A1:B1"),
				"--border-styles", `{"top":{"style":"solid","weight":"hair"}}`,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("explicit bold aliases outrank the style envelope", func(t *testing.T) {
		for _, command := range []string{"+cells-set-style", "+cells-batch-set-style"} {
			for _, flag := range []string{"bold", "font-bold"} {
				t.Run(command+"/"+flag, func(t *testing.T) {
					for _, weight := range []string{"true", "false"} {
						args := []string{"sheets", command, "--spreadsheet-token", spreadsheetToken}
						if command == "+cells-set-style" {
							args = append(args, "--range", qualified("A1"))
						} else {
							ranges, err := json.Marshal([]string{sheetName + "!A1"})
							require.NoError(t, err)
							args = append(args, "--ranges", string(ranges))
						}
						args = append(args, "--"+flag+"="+weight)
						if weight == "false" {
							args = append(args, "--style", `{"font_weight":"bold"}`)
						}
						result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
						require.NoError(t, err)
						result.AssertExitCode(t, 0)
						result, err = clie2e.RunCmd(ctx, clie2e.Request{
							Args: []string{
								"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken,
								"--range", qualified("A1"), "--include", "style",
							},
							DefaultAs: "bot",
						})
						require.NoError(t, err)
						result.AssertExitCode(t, 0)
						require.True(t, gjson.Get(result.Stdout, "data.complete").Bool(), result.Stdout)
						require.False(t, gjson.Get(result.Stdout, "data.has_more").Bool(), result.Stdout)
						require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.returned_cell_count").Int(), result.Stdout)
						cell := gjson.Get(result.Stdout, "data.ranges.0.cells.0.0")
						require.Equal(t, "名称", cell.Get("value").String(), result.Stdout)
						require.True(t, cell.Get("cell_styles").IsObject(), result.Stdout)
						fontWeight := cell.Get("cell_styles.font_weight")
						if weight == "true" {
							require.Equal(t, "bold", fontWeight.String(), "read-back must prove the cell was bold: %s", result.Stdout)
						} else {
							// The read API omits the default normal font_weight.
							// The preceding bold read proves the clear was not a no-op.
							require.True(t, !fontWeight.Exists() || fontWeight.String() == "normal",
								"the explicit false alias must clear bold despite the envelope: %s", result.Stdout)
						}
					}
				})
			}
		}
	})
	t.Run("style vocabulary survives every input carrier", func(t *testing.T) {
		for i, command := range []string{"+cells-set-style", "+cells-batch-set-style", "+cells-set", "+styles-put", "+batch-update"} {
			t.Run(command, func(t *testing.T) {
				// Each carrier changes the size so a no-op cannot pass by reading
				// the style applied by the preceding carrier.
				fontSize := 14 + i
				style := fmt.Sprintf(`{"font":{"bold":true,"size":%d,"color":"#123456"},"h_align":"center","v_align":"center","wrap_text":true,"border_styles":{"all":{"style":"solid","weight":1},"outer":{"style":"solid","weight":1.0},"bottom":{"style":"dashed","weight":"thin","color":"#123456"}}}`, fontSize)
				args := []string{"sheets", command, "--spreadsheet-token", spreadsheetToken}
				switch command {
				case "+cells-set-style":
					args = append(args, "--range", qualified("A1"), "--style", style)
				case "+cells-batch-set-style":
					ranges, err := json.Marshal([]string{qualified("A1")})
					require.NoError(t, err)
					args = append(args, "--ranges", string(ranges), "--styles", style)
				case "+cells-set":
					args = append(args, "--range", qualified("A1"), "--cells", `[[{"value":"名称","cell_styles":`+style+`}]]`)
				case "+styles-put":
					name, err := json.Marshal(sheetName)
					require.NoError(t, err)
					args = append(args, "--styles", `{"styles":[{"name":`+string(name)+`,"cell_styles":[{"range":"A1",`+style[1:]+`]}]}`)
				case "+batch-update":
					name, err := json.Marshal(sheetName)
					require.NoError(t, err)
					args = append(args, "--operations", `[{"shortcut":"+cells-set-style","input":{"sheet_name":`+string(name)+`,"range":"A1","style":`+style+`}}]`)
				}
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot", Yes: command == "+batch-update"})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result, err = clie2e.RunCmd(ctx, clie2e.Request{
					Args:      []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken, "--range", qualified("A1"), "--include", "style"},
					DefaultAs: "bot",
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				cell := gjson.Get(result.Stdout, "data.ranges.0.cells.0.0")
				require.Equal(t, "名称", cell.Get("value").String(), result.Stdout)
				require.Equal(t, "bold", cell.Get("cell_styles.font_weight").String(), result.Stdout)
				require.Equal(t, int64(fontSize), cell.Get("cell_styles.font_size").Int(), result.Stdout)
				require.Equal(t, "#123456", strings.ToLower(cell.Get("cell_styles.font_color").String()), result.Stdout)
				require.Equal(t, "center", cell.Get("cell_styles.horizontal_alignment").String(), result.Stdout)
				require.Equal(t, "middle", cell.Get("cell_styles.vertical_alignment").String(), result.Stdout)
				// The live Sheets read API currently omits word_wrap after a successful
				// write, while all other style fields above round-trip. Dry-run carrier
				// tests assert the request body contains auto-wrap; do not turn this
				// backend readback omission into a client compatibility failure.
				wordWrap := cell.Get("cell_styles.word_wrap")
				require.True(t, !wordWrap.Exists() || wordWrap.String() == "auto-wrap", result.Stdout)
				require.Equal(t, "dashed", cell.Get("border_styles.bottom.style").String(), result.Stdout)
				for _, side := range []string{"top", "left", "right"} {
					require.Equal(t, "solid", cell.Get("border_styles."+side+".style").String(), result.Stdout)
					require.Equal(t, "thin", cell.Get("border_styles."+side+".weight").String(), result.Stdout)
				}
			})
		}
	})
	t.Run("nested and numeric alignment round trip", func(t *testing.T) {
		for _, args := range [][]string{
			{"--style", `{"alignment":{"horizontal":"right","vertical":"bottom","wrapText":false}}`},
			{"--style", `{"hAlign":0.0,"vAlign":0e0}`},
			{"--style", `{"hAlign":1e0,"vAlign":1.0}`},
			{"--style", `{"Alignment":{"horizontal":null,"vertical":"bottom"}}`, "--horizontal-alignment", "right"},
		} {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: append([]string{"sheets", "+cells-set-style", "--spreadsheet-token", spreadsheetToken,
					"--range", qualified("A6")}, args...), DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result, err = clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken,
					"--range", qualified("A6"), "--include", "style"}, DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			style := gjson.Get(result.Stdout, "data.ranges.0.cells.0.0.cell_styles")
			horizontal, vertical := "right", "bottom"
			switch args[1] {
			case `{"hAlign":0.0,"vAlign":0e0}`:
				horizontal, vertical = "left", "top"
			case `{"hAlign":1e0,"vAlign":1.0}`:
				horizontal, vertical = "center", "middle"
			}
			require.Equal(t, horizontal, style.Get("horizontal_alignment").String(), result.Stdout)
			require.Equal(t, vertical, style.Get("vertical_alignment").String(), result.Stdout)
		}
	})
	t.Run("size lists and echoed ranges use exact dimensions", func(t *testing.T) {
		for _, args := range [][]string{
			{"+rows-resize", "--range", "7:8", "--heights", `{"7":44,"8":52}`},
			{"+cols-resize", "--range", "D:E", "--widths", `[96,144]`},
			{"+cols-resize", "--widths", `[{"column":"F","width":88}]`},
		} {
			command := []string{"sheets", args[0], "--spreadsheet-token", spreadsheetToken, "--sheet-name", sheetName}
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append(command, args[1:]...), DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
		}
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+sheet-info", "--spreadsheet-token", spreadsheetToken, "--sheet-name", sheetName,
				"--range", "A7:F8", "--include", "row_heights,col_widths"}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		for path, want := range map[string]int64{
			`data.row_heights.#(rows=="7:7").height`:  44,
			`data.row_heights.#(rows=="8:8").height`:  52,
			`data.column_widths.#(cols=="D:D").width`: 96,
			`data.column_widths.#(cols=="E:E").width`: 144,
			`data.column_widths.#(cols=="F:F").width`: 88,
		} {
			require.Equal(t, want, gjson.Get(result.Stdout, path).Int(), result.Stdout)
		}
	})
	t.Run("sparse cells leave the gap untouched", func(t *testing.T) {
		for _, args := range [][]string{
			{"--range", qualified("A4:C4"), "--cells", `[["old-left","keep-middle","old-right"]]`},
			{"--cells", fmt.Sprintf(`[{"cell":%q,"type":"number","value":42},{"range":%q,"value":"new-right"}]`, qualified("A4"), qualified("C4"))},
		} {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      append([]string{"sheets", "+cells-set", "--spreadsheet-token", spreadsheetToken}, args...),
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
		}
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken, "--range", qualified("A4:C4"), "--include", "value"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		for i, want := range []string{"42", "keep-middle", "new-right"} {
			require.Equal(t, want, gjson.Get(result.Stdout, fmt.Sprintf("data.ranges.0.cells.0.%d.value", i)).String(), result.Stdout)
		}
	})
	t.Run("matching content type annotations retain values and formulas", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+cells-set", "--spreadsheet-token", spreadsheetToken, "--range", qualified("A5:D5"),
				"--cells", `[[{"type":"string","value":"00123","cell_styles":{"number_format":"@","font":{"bold":1}}},{"type":"number","value":0},{"type":"boolean","value":false},{"type":"formula","value":"=SUM(1,2)"}]]`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result, err = clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken,
				"--range", qualified("A5:D5"), "--include", "value,formula,style"}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Equal(t, "00123", gjson.Get(result.Stdout, "data.ranges.0.cells.0.0.value").Value(), result.Stdout)
		require.Equal(t, "bold", gjson.Get(result.Stdout, "data.ranges.0.cells.0.0.cell_styles.font_weight").String(), result.Stdout)
		// Stored numbers and booleans are read as display strings; dry-run
		// tests separately pin their JSON types in the write request.
		require.Equal(t, "0", gjson.Get(result.Stdout, "data.ranges.0.cells.0.1.value").String(), result.Stdout)
		require.Equal(t, "FALSE", gjson.Get(result.Stdout, "data.ranges.0.cells.0.2.value").String(), result.Stdout)
		require.Equal(t, "3", gjson.Get(result.Stdout, "data.ranges.0.cells.0.3.value").String(), result.Stdout)
		require.Contains(t, gjson.Get(result.Stdout, "data.ranges.0.cells.0.3.formula").String(), "SUM", result.Stdout)
	})
	t.Run("nested declarative styles and border attributes round trip", func(t *testing.T) {
		name, err := json.Marshal(sheetName)
		require.NoError(t, err)
		for _, args := range [][]string{
			{"+cells-set", "--range", qualified("A7"), "--cells", `[[{"type":"text","text":"00123"}]]`},
			{"+styles-put", "--styles", `{"styles":[{"name":` + string(name) + `,"cell_styles":[{"range":"A7","style":{"fontWeight":"bold","textAlign":"right","fillColor":"#123456"}}]}]}`},
			{"+cells-set-style", "--range", qualified("A7"), "--border-type", "LEFT_BORDER", "--border-color", "#654321", "--border-weight", "medium"},
		} {
			r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append([]string{"sheets", args[0], "--spreadsheet-token", spreadsheetToken}, args[1:]...), DefaultAs: "bot"})
			require.NoError(t, err)
			r.AssertExitCode(t, 0)
		}
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken, "--range", qualified("A7"), "--include", "value,style"}, DefaultAs: "bot"})
		require.NoError(t, err)
		r.AssertExitCode(t, 0)
		cell := gjson.Get(r.Stdout, "data.ranges.0.cells.0.0")
		require.Equal(t, "00123", cell.Get("value").Value(), r.Stdout)
		require.Equal(t, "bold", cell.Get("cell_styles.font_weight").String(), r.Stdout)
		require.Equal(t, "right", cell.Get("cell_styles.horizontal_alignment").String(), r.Stdout)
		require.Equal(t, "#123456", strings.ToLower(cell.Get("cell_styles.background_color").String()), r.Stdout)
		require.Equal(t, "#654321", strings.ToLower(cell.Get("border_styles.left.color").String()), r.Stdout)
	})
	t.Run("sort direction forms move whole rows", func(t *testing.T) {
		for _, args := range [][]string{
			{"+cells-set", "--range", qualified("A8:C10"), "--cells", `[["b",2,"B"],["a",1,"A"],["c",3,"C"]]`},
			{"+range-sort", "--range", qualified("A8:C10"), "--sort-keys", `[{"column":"B","order":"asc"}]`},
		} {
			r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append([]string{"sheets", args[0], "--spreadsheet-token", spreadsheetToken}, args[1:]...), DefaultAs: "bot"})
			require.NoError(t, err)
			r.AssertExitCode(t, 0)
		}
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken, "--range", qualified("A8:C10"), "--include", "value"}, DefaultAs: "bot"})
		require.NoError(t, err)
		r.AssertExitCode(t, 0)
		for i, label := range []string{"a", "b", "c"} {
			require.Equal(t, label, gjson.Get(r.Stdout, fmt.Sprintf("data.ranges.0.cells.%d.0.value", i)).String(), r.Stdout)
			require.Equal(t, strconv.Itoa(i+1), gjson.Get(r.Stdout, fmt.Sprintf("data.ranges.0.cells.%d.1.value", i)).String(), r.Stdout)
			require.Equal(t, strings.ToUpper(label), gjson.Get(r.Stdout, fmt.Sprintf("data.ranges.0.cells.%d.2.value", i)).String(), r.Stdout)
		}
	})
	t.Run("explicit IDs preserve values through style and format clearing", func(t *testing.T) {
		name, err := json.Marshal(sheetName)
		require.NoError(t, err)
		for _, args := range [][]string{
			{"+table-put", "--sheets", `[{"name":` + string(name) + `,"start_cell":"A12","header":false,"columns":["id"],"data":[["00123"]]}]`},
			{"+styles-put", "--styles", `[{"range":"A12","style":{"font_weight":"bold"}}]`},
			{"+rows-resize", "--start-row", "12", "--end-row", "13", "--height", "55"},
			{"+cells-batch-clear", "--ranges", `["A12"]`, "--scope", "formats", "--yes"},
		} {
			args = append([]string{"sheets", args[0], "--spreadsheet-token", spreadsheetToken, "--sheet-id", explicitSheetID}, args[1:]...)
			r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			r.AssertExitCode(t, 0)
		}
		r, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken, "--sheet-id", explicitSheetID, "--range", "A12", "--include", "value,style"}, DefaultAs: "bot"})
		require.NoError(t, err)
		r.AssertExitCode(t, 0)
		require.Equal(t, "00123", gjson.Get(r.Stdout, "data.ranges.0.cells.0.0.value").Value())
		require.NotEqual(t, "bold", gjson.Get(r.Stdout, "data.ranges.0.cells.0.0.cell_styles.font_weight").String())
	})
	t.Run("copy aliases retain the copied values", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+range-copy", "--spreadsheet-token", spreadsheetToken,
				"--source-sheet-name", sheetName, "--range", "A1:B1", "--copy-to-range", "G1", "--paste-type", "values"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result, err = clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken,
				"--range", qualified("G1:H1"), "--include", "value"}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		for i, want := range []string{"名称", "数量"} {
			require.Equal(t, want, gjson.Get(result.Stdout, fmt.Sprintf("data.ranges.0.cells.0.%d.value", i)).String(), result.Stdout)
		}
	})
	t.Run("agreeing formula inputs write and evaluate the formula", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-set", "--spreadsheet-token", spreadsheetToken,
				"--range", qualified("A2"), "--cells", `[[{"formula":"=SUM(1,2)"}]]`,
				"--formula", "=SUM(1,2)", "--formulas", "=SUM(1,2)",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result, err = clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-get", "--spreadsheet-token", spreadsheetToken,
				"--range", qualified("A2"), "--include", "value,formula",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Equal(t, "3", gjson.Get(result.Stdout, "data.ranges.0.cells.0.0.value").String(), result.Stdout)
		require.Contains(t, gjson.Get(result.Stdout, "data.ranges.0.cells.0.0.formula").String(), "SUM", result.Stdout)
	})
}

// scalarsIn flattens every leaf of a JSON result into its string form, so a
// test can assert a value came back without also pinning where in the envelope
// the server chose to put it.
func scalarsIn(v gjson.Result) []string {
	var out []string
	var walk func(gjson.Result)
	walk = func(r gjson.Result) {
		if r.IsObject() || r.IsArray() {
			r.ForEach(func(_, child gjson.Result) bool {
				walk(child)
				return true
			})
			return
		}
		out = append(out, r.String())
	}
	walk(v)
	return out
}
