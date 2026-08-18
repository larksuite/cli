// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"github.com/larksuite/cli/shortcuts/common"
)

// Shortcuts returns all lark-sheets shortcuts. The list is grouped by
// canonical skill to mirror the sheet-skill-spec layout
// (lark_sheet_workbook → lark_sheet_float_image).
//
// Any shortcut whose command is registered in data/flag-schemas.json gets a
// PrintFlagSchema closure attached, so the framework can serve
// `--print-schema --flag-name <name>` locally.
func Shortcuts() []common.Shortcut {
	all := shortcutList()
	// Gate on the codegen'd command set (flag_schemas_gen.go) so registration
	// — which runs on every CLI invocation — does not parse the 256KB
	// flag-schemas.json. The blob is unmarshaled lazily (printFlagSchemaFor /
	// the validate fast-path) only when actually needed.
	for i := range all {
		if _, ok := commandsWithSchema[all[i].Command]; ok {
			all[i].PrintFlagSchema = printFlagSchemaFor(all[i].Command)
		}
		// Accept the highest-frequency locator misspelling through the common
		// declarative alias contract. Copy the flag slice before decorating it:
		// shortcut values are package globals and Shortcuts may be called more
		// than once in tests or embedders.
		all[i].Flags = withSpreadsheetTokenAlias(all[i].Flags)
		// +chart-create grows --print-example (minimal per-type --properties
		// templates) — the biggest --print-schema consumer in eval traces.
		if all[i].Command == "+chart-create" {
			all[i].PostMount = withChartPrintExample(all[i].PostMount)
		}
		// Sheets-scoped flag ergonomics (unknown-flag hints with the valid
		// flags inlined, enum vocabulary normalization) ride the existing
		// PostMount composition, so no other domain's behavior shifts.
		all[i].PostMount = withFlagErgonomics(all[i].PostMount)
	}
	return all
}

func withSpreadsheetTokenAlias(flags []common.Flag) []common.Flag {
	for i := range flags {
		if flags[i].Name != "spreadsheet-token" {
			continue
		}
		decorated := append([]common.Flag(nil), flags...)
		decorated[i].Aliases = append(append([]string(nil), decorated[i].Aliases...), "token")
		return decorated
	}
	return flags
}

func shortcutList() []common.Shortcut {
	return []common.Shortcut{
		// lark_sheet_workbook
		WorkbookInfo,
		RevisionGet,
		SheetList,
		SheetCreate,
		SheetDelete,
		SheetRename,
		SheetMove,
		SheetCopy,
		SheetHide,
		SheetUnhide,
		SheetSetTabColor,
		SheetShowGridline,
		SheetHideGridline,
		WorkbookCreate,
		WorkbookExport,
		WorkbookImport,

		// lark_sheet_sheet_structure
		SheetInfo,
		DimInsert,
		DimDelete,
		DimHide,
		DimUnhide,
		DimFreeze,
		DimGroup,
		DimUngroup,
		DimMove,

		// lark_sheet_changeset
		ChangesetGet,

		// lark_sheet_read_data
		CellsGet,
		CsvGet,
		DropdownGet,
		TableGet,

		// lark_sheet_search_replace
		CellsSearch,
		CellsReplace,

		// lark_sheet_formula_verify
		FormulaVerify,

		// lark_sheet_write_cells
		CellsSet,
		CellsSetStyle,
		CellsSetImage,
		CsvPut,
		DropdownSet,
		TablePut,

		// lark_sheet_range_operations
		CellsClear,
		CellsMerge,
		CellsUnmerge,
		RowsResize,
		ColsResize,
		RangeMove,
		RangeCopy,
		RangeFill,
		RangeSort,

		// Object list (one read shortcut per object skill)
		ChartList,
		PivotList,
		CondFormatList,
		FilterList,
		FilterViewList,
		SparklineList,
		FloatImageList,

		// Object CRUD (3 per skill)
		ChartCreate, ChartUpdate, ChartDelete,
		PivotCreate, PivotUpdate, PivotDelete,
		CondFormatCreate, CondFormatUpdate, CondFormatDelete,
		FilterCreate, FilterUpdate, FilterDelete,
		FilterViewCreate, FilterViewUpdate, FilterViewDelete,
		SparklineCreate, SparklineUpdate, SparklineDelete,
		FloatImageCreate, FloatImageUpdate, FloatImageDelete,

		// lark_sheet_styles_put
		StylesPut,

		// lark_sheet_batch_update
		BatchUpdate,
		CellsBatchSetStyle,
		CellsBatchClear,
		DropdownUpdate,
		DropdownDelete,

		// lark_sheet_history
		HistoryList,
		HistoryRevert,
		HistoryRevertStatus,
	}
}
