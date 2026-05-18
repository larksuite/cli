// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_write_cells ───────────────────────────────────────────
//
// Wraps:
//   - set_cell_range     (powers +cells-set / +cells-set-style /
//                        +dropdown-set / +dropdown-update / +dropdown-delete)
//   - set_range_from_csv (powers +csv-put)
//
// +cells-set-image is a `cli_only_derivative` shortcut (needs a local file
// upload before calling set_cell_range); it lives in the cli-only batch
// where the upload helper is shared with +workbook-create / +dim-move /
// +workbook-export.
//
// All set_cell_range-backed shortcuts construct a cells matrix whose
// dimensions exactly match the target range — the tool errors on mismatch.

// CellsSet wraps set_cell_range with raw --data: caller provides the cells
// matrix (and any optional copy_to_range / resize_* fields) as JSON.
var CellsSet = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-set",
	Description: "Write values / formulas / styles / comments / data validation / embed-image to a cell range.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "range", Required: true, Desc: "target A1 range (e.g. A1:C10); cells dimensions must match"},
		common.Flag{Name: "cells", Input: []string{common.File, common.Stdin}, Required: true,
			Desc: "JSON body: { \"cells\": [[{value|formula|cell_styles|...}, ...]], optional copy_to_range / resize_width / resize_height }"},
		common.Flag{Name: "allow-overwrite", Type: "bool", Default: "true", Desc: "allow overwriting non-empty cells (default true)"},
		common.Flag{Name: "max-cells", Type: "int", Default: "50000", Hidden: true, Desc: "anti-burst cells write cap"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("range")) == "" {
			return common.FlagErrorf("--range is required")
		}
		body, err := requireJSONObject(runtime, "cells")
		if err != nil {
			return err
		}
		if _, ok := body["cells"]; !ok {
			return common.FlagErrorf("--cells must include a \"cells\" field")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := cellsSetInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "set_cell_range", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := cellsSetInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func cellsSetInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) (map[string]interface{}, error) {
	body, err := requireJSONObject(runtime, "cells")
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id": token,
		"range":    strings.TrimSpace(runtime.Str("range")),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	// --cells fields override any of these except the core selectors.
	for k, v := range body {
		switch k {
		case "excel_id", "range", "sheet_id", "sheet_name":
			// reserved for flat flags
		default:
			input[k] = v
		}
	}
	if !runtime.Bool("allow-overwrite") {
		input["allow_overwrite"] = false
	}
	return input, nil
}

// CellsSetStyle stamps a single style block across every cell in --range.
// Style is composed from a dozen flat flags (background-color, font-color,
// font-size, font-style, font-weight, font-line, horizontal-alignment,
// vertical-alignment, word-wrap, number-format) plus --border-styles for
// the only field that still needs a nested object. At least one flag must
// be set.
var CellsSetStyle = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-set-style",
	Description: "Apply style flags to every cell in a range (values / formulas untouched).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(
		append(publicSheetFlags(),
			common.Flag{Name: "range", Required: true, Desc: "target A1 range (e.g. A1:B2)"}),
		styleFlatFlags()...,
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		r := strings.TrimSpace(runtime.Str("range"))
		if r == "" {
			return common.FlagErrorf("--range is required")
		}
		if _, _, err := rangeDimensions(r); err != nil {
			return common.FlagErrorf("--range %q: %v", r, err)
		}
		if err := requireAnyStyleFlag(runtime); err != nil {
			return err
		}
		if _, err := borderStylesFromFlag(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := cellsSetStyleInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "set_cell_range", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := cellsSetStyleInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func cellsSetStyleInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) (map[string]interface{}, error) {
	rangeStr := strings.TrimSpace(runtime.Str("range"))
	rows, cols, err := rangeDimensions(rangeStr)
	if err != nil {
		return nil, common.FlagErrorf("--range %q: %v", rangeStr, err)
	}
	cellStyle := buildCellStyleFromFlags(runtime)
	borderStyles, err := borderStylesFromFlag(runtime)
	if err != nil {
		return nil, err
	}
	cells := make([][]interface{}, rows)
	for r := range cells {
		row := make([]interface{}, cols)
		for c := range row {
			cell := map[string]interface{}{}
			if len(cellStyle) > 0 {
				cell["cell_styles"] = cellStyle
			}
			if borderStyles != nil {
				cell["border_styles"] = borderStyles
			}
			row[c] = cell
		}
		cells[r] = row
	}
	input := map[string]interface{}{
		"excel_id": token,
		"range":    rangeStr,
		"cells":    cells,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

// CsvPut wraps set_range_from_csv: dump a CSV blob into a sheet, only writing
// plain values. Use +cells-set for anything richer (formula / style / note).
var CsvPut = common.Shortcut{
	Service:     "sheets",
	Command:     "+csv-put",
	Description: "Paste RFC-4180 CSV into a sheet at --start-cell (plain values only, auto-expands sheet if needed).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "csv", Input: []string{common.File, common.Stdin}, Required: true,
			Desc: "CSV text (RFC 4180); supports @file or stdin via -"},
		common.Flag{Name: "start-cell", Default: "A1", Required: true, Desc: "single A1 anchor cell, e.g. A1 / B5"},
		common.Flag{Name: "allow-overwrite", Type: "bool", Default: "true",
			Desc: "allow overwriting non-empty cells (default true); false errors if any target cell is non-empty"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("csv")) == "" {
			return common.FlagErrorf("--csv is required")
		}
		anchor := strings.TrimSpace(runtime.Str("start-cell"))
		if anchor == "" {
			return common.FlagErrorf("--start-cell is required")
		}
		if _, _, ok := splitCellRef(anchor); !ok {
			return common.FlagErrorf("--start-cell %q must be a single cell ref (e.g. A1)", anchor)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "set_range_from_csv", csvPutInput(runtime, token, sheetID, sheetName))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "set_range_from_csv", csvPutInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func csvPutInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{
		"excel_id":   token,
		"csv":        runtime.Str("csv"),
		"start_cell": strings.TrimSpace(runtime.Str("start-cell")),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if !runtime.Bool("allow-overwrite") {
		input["allow_overwrite"] = false
	}
	return input
}

// ─── +dropdown-* (set_cell_range via data_validation) ─────────────────
//
// All three dropdown shortcuts stamp a `data_validation` block on every cell
// of the target range(s). set / update / delete differ in (a) how many
// ranges they accept and (b) whether the block is populated or null.

// DropdownSet places a single dropdown on one range.
var DropdownSet = common.Shortcut{
	Service:     "sheets",
	Command:     "+dropdown-set",
	Description: "Attach a dropdown / data-validation list to every cell in --range.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "range", Required: true, Desc: "target A1 range (e.g. A2:A100)"},
		common.Flag{Name: "options", Input: []string{common.File, common.Stdin}, Required: true,
			Desc: "options JSON array (e.g. [\"opt1\",\"opt2\"]); ≤500 items, ≤100 chars each, no commas"},
		common.Flag{Name: "colors", Input: []string{common.File, common.Stdin},
			Desc: "optional RGB hex array (e.g. [\"#1FB6C1\",\"#F006C2\"]); length must equal --options"},
		common.Flag{Name: "multiple", Type: "bool", Desc: "enable multi-select; default false"},
		common.Flag{Name: "highlight", Type: "bool", Desc: "color-highlight options; default false"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		r := strings.TrimSpace(runtime.Str("range"))
		if r == "" {
			return common.FlagErrorf("--range is required")
		}
		if _, _, err := rangeDimensions(r); err != nil {
			return common.FlagErrorf("--range %q: %v", r, err)
		}
		if _, err := validateDropdownOptionsColors(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := dropdownSetInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "set_cell_range", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := dropdownSetInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func dropdownSetInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) (map[string]interface{}, error) {
	validation, err := buildDropdownValidation(runtime)
	if err != nil {
		return nil, err
	}
	rangeStr := strings.TrimSpace(runtime.Str("range"))
	rows, cols, err := rangeDimensions(rangeStr)
	if err != nil {
		return nil, common.FlagErrorf("--range %q: %v", rangeStr, err)
	}
	cells := fillCellsMatrix(rows, cols, map[string]interface{}{"data_validation": validation})
	input := map[string]interface{}{
		"excel_id": token,
		"range":    rangeStr,
		"cells":    cells,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

// NOTE: +dropdown-update and +dropdown-delete were originally drafted here
// but moved to lark_sheet_batch_update (B7) per the spec: multi-range
// dropdown CRUD now goes through batch_update for atomicity. They'll land in
// the batch_update file alongside +cells-batch-set-style.

// ─── shared dropdown helpers ──────────────────────────────────────────

// buildDropdownValidation packs --options / --colors / --multiple / --highlight
// into the data_validation block expected by set_cell_range.
func buildDropdownValidation(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	options, err := requireJSONArray(runtime, "options")
	if err != nil {
		return nil, err
	}
	dv := map[string]interface{}{
		"type":   "list",
		"values": options,
	}
	if runtime.Str("colors") != "" {
		colors, err := requireJSONArray(runtime, "colors")
		if err != nil {
			return nil, err
		}
		if len(colors) != len(options) {
			return nil, common.FlagErrorf("--colors length (%d) must equal --options length (%d)", len(colors), len(options))
		}
		dv["colors"] = colors
	}
	if runtime.Bool("multiple") {
		dv["multiple_values"] = true
	}
	if runtime.Bool("highlight") {
		dv["highlight_options"] = true
	}
	return dv, nil
}

// validateDropdownOptionsColors validates --options is a JSON array and that
// --colors (when set) has matching length. Used by +dropdown-set Validate.
func validateDropdownOptionsColors(runtime *common.RuntimeContext) (int, error) {
	options, err := requireJSONArray(runtime, "options")
	if err != nil {
		return 0, err
	}
	if runtime.Str("colors") != "" {
		colors, err := requireJSONArray(runtime, "colors")
		if err != nil {
			return 0, err
		}
		if len(colors) != len(options) {
			return 0, common.FlagErrorf("--colors length (%d) must equal --options length (%d)", len(colors), len(options))
		}
	}
	return len(options), nil
}

// ─── range parsing helpers ────────────────────────────────────────────

// rangeDimensions parses an A1 range like "A1:C5" / "A1" / "sheet1!B2:D10"
// and returns its row / column counts. Errors on non-rectangular forms like
// "A:C" (whole-column) or "3:6" (whole-row) — those need a row/col total
// from get_sheet_structure, outside the scope of pure local parsing.
func rangeDimensions(rangeStr string) (rows, cols int, err error) {
	if idx := strings.Index(rangeStr, "!"); idx >= 0 {
		rangeStr = rangeStr[idx+1:]
	}
	rangeStr = strings.TrimSpace(rangeStr)
	if rangeStr == "" {
		return 0, 0, fmt.Errorf("empty range")
	}
	parts := strings.SplitN(rangeStr, ":", 2)
	if len(parts) == 1 {
		// single cell, e.g. "A1"
		if _, _, ok := splitCellRef(parts[0]); !ok {
			return 0, 0, fmt.Errorf("invalid cell ref %q", parts[0])
		}
		return 1, 1, nil
	}
	startCol, startRow, ok1 := splitCellRef(parts[0])
	endCol, endRow, ok2 := splitCellRef(parts[1])
	if !ok1 || !ok2 {
		return 0, 0, fmt.Errorf("unsupported range form %q (need rectangular A1:B2)", rangeStr)
	}
	if endRow < startRow || endCol < startCol {
		return 0, 0, fmt.Errorf("end %q must be at or after start %q", parts[1], parts[0])
	}
	return endRow - startRow + 1, endCol - startCol + 1, nil
}

// splitCellRef parses "A1" → (col=0, row=0, true). Returns false for any
// non-rectangular form (pure column "A", pure row "1", invalid chars).
func splitCellRef(s string) (col, row int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	var colEnd int
	for i, r := range s {
		if r >= '0' && r <= '9' {
			colEnd = i
			break
		}
		colEnd = i + 1
	}
	if colEnd == 0 || colEnd == len(s) {
		return 0, 0, false
	}
	col = letterToColumnIndex(s[:colEnd])
	if col < 0 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(s[colEnd:])
	if err != nil || n < 1 {
		return 0, 0, false
	}
	return col, n - 1, true
}

// letterToColumnIndex converts spreadsheet letter notation to a 0-based
// column index ("A" → 0, "Z" → 25, "AA" → 26). Returns -1 on bad input.
func letterToColumnIndex(letters string) int {
	letters = strings.ToUpper(strings.TrimSpace(letters))
	if letters == "" {
		return -1
	}
	n := 0
	for _, c := range letters {
		if c < 'A' || c > 'Z' {
			return -1
		}
		n = n*26 + int(c-'A'+1)
	}
	return n - 1
}

// fillCellsMatrix returns a rows×cols matrix where every cell is the same
// (shallow-copied) prototype map. Use for fan-out shortcuts that stamp a
// single attribute (style / data_validation) across an entire range.
func fillCellsMatrix(rows, cols int, prototype map[string]interface{}) [][]interface{} {
	cells := make([][]interface{}, rows)
	for r := range cells {
		row := make([]interface{}, cols)
		for c := range row {
			cell := make(map[string]interface{}, len(prototype))
			for k, v := range prototype {
				cell[k] = v
			}
			row[c] = cell
		}
		cells[r] = row
	}
	return cells
}

// ─── +cells-set-image (cli_only_derivative) ──────────────────────────
//
// The backing tool (set_cell_range) is in mcp-tools.json, but the CLI
// shortcut also needs a local-file upload before it can call the tool.
// That extra step doesn't fit the One-OpenAPI dispatcher, so the spec
// marks this shortcut cli_only_derivative — the CLI uploads the image
// to drive (parent_type=sheet_image) and then writes the returned
// file_token into the target cell via callTool(set_cell_range) with a
// rich_text embed-image entry.

// CellsSetImage uploads a local image to drive (parent_type=sheet_image,
// parent_node=spreadsheet token) and then writes a rich_text embed-image
// into the target single-cell range via the set_cell_range tool.
var CellsSetImage = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-set-image",
	Description: "Embed a local image into a single cell (uploads via drive, then set_cell_range with rich_text embed-image).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "drive:file:upload"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "range", Required: true, Desc: "single target cell (e.g. A1; start/end must equal)"},
		common.Flag{Name: "image", Required: true, Desc: "local image path (PNG/JPEG/JPG/GIF/BMP/JFIF/EXIF/TIFF/BPG/HEIC)"},
		common.Flag{Name: "name", Desc: "uploaded file name (with extension); defaults to basename(--image)"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		r := strings.TrimSpace(runtime.Str("range"))
		if r == "" {
			return common.FlagErrorf("--range is required")
		}
		rows, cols, err := rangeDimensions(r)
		if err != nil {
			return common.FlagErrorf("--range %q: %v", r, err)
		}
		if rows != 1 || cols != 1 {
			return common.FlagErrorf("--range %q must be exactly one cell (got %d×%d)", r, rows, cols)
		}
		if strings.TrimSpace(runtime.Str("image")) == "" {
			return common.FlagErrorf("--image is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		imgPath := strings.TrimSpace(runtime.Str("image"))
		fileName := strings.TrimSpace(runtime.Str("name"))
		if fileName == "" {
			fileName = filepath.Base(imgPath)
		}
		setCellBody, _ := buildToolBody("set_cell_range", map[string]interface{}{
			"excel_id": token,
			"range":    strings.TrimSpace(runtime.Str("range")),
			"sheet_id": sheetSelectorPlaceholder(sheetID, sheetName),
			"cells": [][]interface{}{{map[string]interface{}{
				"rich_text": []map[string]interface{}{{
					"type":             "embed-image",
					"attachment_token": "<file_token>",
					"attachment_name":  fileName,
				}},
			}}},
		})
		return common.NewDryRunAPI().
			POST("/open-apis/drive/v1/medias/upload_all").
			Desc("upload local image to drive (parent_type=sheet_image)").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": "sheet_image",
				"parent_node": token,
				"size":        "<file_size>",
				"file":        "@" + imgPath,
			}).
			POST(toolInvokePath(token, ToolKindWrite)).
			Desc("embed file_token into the cell via set_cell_range").
			Body(setCellBody)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		imgPath := strings.TrimSpace(runtime.Str("image"))
		fileName := strings.TrimSpace(runtime.Str("name"))
		if fileName == "" {
			fileName = filepath.Base(imgPath)
		}
		info, err := runtime.FileIO().Stat(imgPath)
		if err != nil {
			return common.WrapInputStatError(err)
		}
		fileToken, err := common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   imgPath,
			FileName:   fileName,
			FileSize:   info.Size(),
			ParentType: "sheet_image",
			ParentNode: &token,
		})
		if err != nil {
			return err
		}

		setCellInput := map[string]interface{}{
			"excel_id": token,
			"range":    strings.TrimSpace(runtime.Str("range")),
			"cells": [][]interface{}{{map[string]interface{}{
				"rich_text": []map[string]interface{}{{
					"type":             "embed-image",
					"attachment_token": fileToken,
					"attachment_name":  fileName,
				}},
			}}},
		}
		sheetSelectorForToolInput(setCellInput, sheetID, sheetName)
		setCellOut, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", setCellInput)
		if err != nil {
			return fmt.Errorf("image uploaded (file_token=%s) but cell write failed: %w", fileToken, err)
		}
		runtime.Out(map[string]interface{}{
			"file_token":     fileToken,
			"file_name":      fileName,
			"set_cell_range": setCellOut,
		}, nil)
		return nil
	},
	Tips: []string{
		"--range must be a single cell. The uploaded image becomes a cell-internal embed; use +float-image-create for floating images.",
	},
}
