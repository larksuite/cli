// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── object list shortcuts ────────────────────────────────────────────
//
// Seven object-collection skills each expose a single "list" read shortcut
// that lives next to their CRUD siblings (chart / pivot / cond-format /
// filter / filter-view / sparkline / float-image). All seven share the
// exact same shape — public sheet selector + optional --<id> filter — so
// they're declared via newObjectListShortcut.
//
// +filter-view-list is `cli_status: cli-only`, but the underlying tool
// get_filter_view_objects is in mcp-tools.json and dispatches through the
// same One-OpenAPI endpoint as everything else; no special path needed.

// objectListSpec describes a single list-style read shortcut.
type objectListSpec struct {
	command     string // CLI command, e.g. "+chart-list"
	description string // one-liner for --help
	toolName    string // MCP tool name, e.g. "get_chart_objects"

	// Optional id filter. Empty filterFlag → no filter flag exposed.
	filterFlag  string // CLI flag name (without leading --), e.g. "chart-id"
	filterField string // tool input key, e.g. "chart_id"
}

func newObjectListShortcut(spec objectListSpec) common.Shortcut {
	flags := flagsFor(spec.command)
	return common.Shortcut{
		Service:     "sheets",
		Command:     spec.command,
		Description: spec.description,
		Risk:        "read",
		Scopes:      []string{"sheets:spreadsheet:read"},
		AuthTypes:   []string{"user", "bot"},
		HasFormat:   true,
		Flags:       flags,
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			if _, err := resolveSpreadsheetToken(runtime); err != nil {
				return err
			}
			_, _, err := resolveSheetSelector(runtime)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			return invokeToolDryRun(token, ToolKindRead, spec.toolName, objectListInput(runtime, token, sheetID, sheetName, spec))
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
			out, err := callTool(ctx, runtime, token, ToolKindRead, spec.toolName, objectListInput(runtime, token, sheetID, sheetName, spec))
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

func objectListInput(runtime *common.RuntimeContext, token, sheetID, sheetName string, spec objectListSpec) map[string]interface{} {
	input := map[string]interface{}{"excel_id": token}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if spec.filterFlag != "" {
		if v := strings.TrimSpace(runtime.Str(spec.filterFlag)); v != "" {
			input[spec.filterField] = v
		}
	}
	return input
}

// ─── shortcut declarations ────────────────────────────────────────────

// ChartList — list charts on a sheet (optionally filtered to one chart_id).
var ChartList = newObjectListShortcut(objectListSpec{
	command:     "+chart-list",
	description: "List charts on a sheet, optionally filtered to a single chart_id.",
	toolName:    "get_chart_objects",
	filterFlag:  "chart-id",
	filterField: "chart_id",
})

// PivotList — list pivot tables on a sheet.
var PivotList = newObjectListShortcut(objectListSpec{
	command:     "+pivot-list",
	description: "List pivot tables on a sheet, optionally filtered to a single pivot_table_id.",
	toolName:    "get_pivot_table_objects",
	filterFlag:  "pivot-table-id",
	filterField: "pivot_table_id",
})

// CondFormatList — list conditional format rules. CLI's --rule-id maps to
// the tool's conditional_format_id (CLI uses the shorter common term).
var CondFormatList = newObjectListShortcut(objectListSpec{
	command:     "+cond-format-list",
	description: "List conditional format rules on a sheet, optionally filtered to a single rule.",
	toolName:    "get_conditional_format_objects",
	filterFlag:  "rule-id",
	filterField: "conditional_format_id",
})

// FilterList — list active sheet-level filters. No id filter because each
// sheet carries at most one filter.
var FilterList = newObjectListShortcut(objectListSpec{
	command:     "+filter-list",
	description: "List active sheet-level filters across the workbook (or one sheet).",
	toolName:    "get_filter_objects",
})

// FilterViewList — list filter views on a sheet. `cli-only` skill (not
// exposed as MCP tool catalog), but the tool itself is dispatched through
// the same One-OpenAPI endpoint.
var FilterViewList = newObjectListShortcut(objectListSpec{
	command:     "+filter-view-list",
	description: "List filter views on a sheet, optionally filtered to a single view_id.",
	toolName:    "get_filter_view_objects",
	filterFlag:  "view-id",
	filterField: "view_id",
})

// SparklineList — list sparkline groups on a sheet. The tool also accepts
// a per-sparkline id (`sparkline_id`); CLI exposes the higher-level
// --group-id which is what callers usually care about.
var SparklineList = newObjectListShortcut(objectListSpec{
	command:     "+sparkline-list",
	description: "List sparkline groups on a sheet, optionally filtered by group_id.",
	toolName:    "get_sparkline_objects",
	filterFlag:  "group-id",
	filterField: "group_id",
})

// FloatImageList — list floating images on a sheet (vs. embedded
// cell-images which live in cell metadata).
var FloatImageList = newObjectListShortcut(objectListSpec{
	command:     "+float-image-list",
	description: "List floating images on a sheet, optionally filtered to a single float_image_id.",
	toolName:    "get_float_image_objects",
	filterFlag:  "float-image-id",
	filterField: "float_image_id",
})
