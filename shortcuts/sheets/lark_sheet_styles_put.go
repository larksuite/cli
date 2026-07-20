// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── +styles-put ──────────────────────────────────────────────────────
//
// Declarative visual spec for EXISTING spreadsheets. Eval attribution
// showed ~73% of real +batch-update calls were pure formatting finishers
// (style stamps + merges + resizes + freeze) hand-assembled as imperative
// operations arrays — the top error surface. +styles-put replaces that
// with the {styles:[...]} protocol already shared by +workbook-create /
// +table-put --styles (identical vocabulary, parsed by the same
// parseWorkbookCreateStyleItem), applied to a live workbook and expanded
// client-side into ONE atomic batch_update.
//
// Per-sheet expansion order (server behavior verified live: style stamps
// over merged regions are allowed — the top-left-only restriction applies
// to value writes, not styles):
//
//	cell_merges → cell_styles → row_sizes → col_sizes → freeze
var StylesPut = common.Shortcut{
	Service:     "sheets",
	Command:     "+styles-put",
	Description: "Apply one declarative visual spec (styles/merges/row-col sizes/freeze) to existing sheets in one atomic batch.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+styles-put"),
	Tips: []string{
		`Example: lark-cli sheets +styles-put --url <URL> --styles '{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1:F1","font_weight":"bold"}],"freeze":{"rows":1}}]}'`,
		"Same --styles vocabulary as +workbook-create / +table-put; one item per target sheet, name = the real sheet name.",
		"Style stamps are safe to re-run; the whole spec goes out as one atomic batch.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		_, err = stylesPutOperations(runtime, token)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		ops, _ := stylesPutOperations(runtime, token)
		return invokeToolDryRun(token, ToolKindWrite, "batch_update", map[string]interface{}{
			"excel_id":   token,
			"operations": ops,
		})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		ops, err := stylesPutOperations(runtime, token)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "batch_update", map[string]interface{}{
			"excel_id":   token,
			"operations": ops,
		})
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// stylesPutOperations parses --styles ({styles:[...]}, one item per target
// sheet) and expands it into the MCP batch_update operations array. Reuses
// the shared workbook-create style item parser, so field validation, alias
// normalization (border "all" shorthand, style vocabulary) and the
// aggregate-all-issues error shape are identical across the three --styles
// carriers.
func stylesPutOperations(runtime flagView, token string) ([]interface{}, error) {
	if strings.TrimSpace(runtime.Str("styles")) == "" {
		return nil, sheetsValidationForFlag("styles", "--styles is required")
	}
	v, err := parseJSONFlag(runtime, "styles")
	if err != nil {
		return nil, err
	}
	items, err := parseWorkbookCreateStylesItems(v)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, sheetsValidationForFlag("styles", "--styles.styles must be a non-empty array (one item per target sheet)")
	}
	var probs []error
	type sheetSpec struct {
		name    string
		payload *workbookCreateStylePayload
	}
	specs := make([]sheetSpec, 0, len(items))
	seenName := map[string]bool{}
	for i, item := range items {
		path := fmt.Sprintf("--styles.styles[%d]", i)
		name, _ := item["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			probs = append(probs, common.ValidationErrorf("%s.name is required (the real sheet name; check +workbook-info)", path))
			continue
		}
		if seenName[name] {
			probs = append(probs, common.ValidationErrorf("%s.name %q appears twice; merge the two items", path, name))
			continue
		}
		seenName[name] = true
		payload, itemProbs := parseWorkbookCreateStyleItem(item, path)
		if len(itemProbs) > 0 {
			probs = append(probs, itemProbs...)
			continue
		}
		specs = append(specs, sheetSpec{name: name, payload: payload})
	}
	if err := joinStyleValidationErrors(probs); err != nil {
		return nil, err
	}

	ops := make([]interface{}, 0, len(specs)*4)
	var totalCells int64
	appendVisual := func(name string, op workbookCreateStyleOp) {
		input, toolName := workbookCreateVisualOpInput(token, "", name, op)
		if toolName == "" {
			return
		}
		ops = append(ops, map[string]interface{}{"tool_name": toolName, "input": input})
	}
	for _, spec := range specs {
		// merges first so subsequent style stamps see the final grid.
		for _, m := range spec.payload.CellMerges {
			appendVisual(spec.name, workbookCreateStyleOp{Kind: "cell_merge", Range: m.Range, MergeType: m.MergeType})
		}
		for _, cs := range spec.payload.CellStyles {
			rows, cols, err := rangeDimensions(cs.Range)
			if err != nil {
				return nil, sheetsValidationForFlag("styles", "cell_styles range %q: %v", cs.Range, err)
			}
			if err := checkStampMatrixBudget("styles", cs.Range, rows, cols); err != nil {
				return nil, err
			}
			totalCells += int64(rows) * int64(cols)
			if err := checkBatchStampBudget(totalCells); err != nil {
				return nil, err
			}
			ops = append(ops, map[string]interface{}{
				"tool_name": "set_cell_range",
				"input": map[string]interface{}{
					"excel_id":   token,
					"sheet_name": spec.name,
					"range":      stripSheetPrefix(cs.Range),
					"cells":      fillCellsMatrix(rows, cols, cs.Style),
				},
			})
		}
		for _, rs := range spec.payload.RowSizes {
			appendVisual(spec.name, workbookCreateStyleOp{Kind: "row_size", Range: rs.Range, ResizeType: rs.ResizeType, Size: rs.Size})
		}
		for _, csz := range spec.payload.ColSizes {
			appendVisual(spec.name, workbookCreateStyleOp{Kind: "col_size", Range: csz.Range, ResizeType: csz.ResizeType, Size: csz.Size})
		}
		if f := spec.payload.Freeze; f != nil {
			if f.Rows > 0 {
				appendVisual(spec.name, workbookCreateStyleOp{Kind: "freeze_rows", Size: f.Rows})
			}
			if f.Cols > 0 {
				appendVisual(spec.name, workbookCreateStyleOp{Kind: "freeze_cols", Size: f.Cols})
			}
		}
	}
	if len(ops) > maxBatchOperations {
		return nil, sheetsValidationForFlag("styles",
			"--styles expands to %d operations, over the %d cap; merge adjacent same-style ranges first, then split the spec into several +styles-put calls",
			len(ops), maxBatchOperations)
	}
	return ops, nil
}

// stripSheetPrefix drops an optional "Sheet!"-style prefix from an A1 range:
// the target sheet is already carried by the spec item's name, and the
// batch sub-op input names the sheet separately.
func stripSheetPrefix(rangeStr string) string {
	if idx := strings.Index(rangeStr, "!"); idx >= 0 {
		return strings.TrimSpace(rangeStr[idx+1:])
	}
	return strings.TrimSpace(rangeStr)
}
