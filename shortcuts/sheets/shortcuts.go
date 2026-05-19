// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all lark-sheets shortcuts. The list is grouped by
// canonical skill to mirror the sheet-skill-spec layout
// (lark_sheet_workbook → lark_sheet_float_image).
//
// Any shortcut whose command is registered in data/flag-schemas.json gets a
// PrintFlagSchema closure attached, so the framework can serve
// `--print-schema --flag-name <name>` locally.
func Shortcuts() []common.Shortcut {
	all := shortcutList()
	withSchema := commandsWithFlagSchema()
	for i := range all {
		if _, ok := withSchema[all[i].Command]; ok {
			all[i].PrintFlagSchema = printFlagSchemaFor(all[i].Command)
		}
	}
	return all
}

func shortcutList() []common.Shortcut {
	return []common.Shortcut{
		// lark_sheet_workbook
		WorkbookInfo,
		SheetCreate,
		SheetDelete,
		SheetRename,
		SheetMove,
		SheetCopy,
		SheetHide,
		SheetUnhide,
		SheetSetTabColor,
		WorkbookCreate,
		WorkbookExport,

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

		// lark_sheet_read_data
		CellsGet,
		CsvGet,
		DropdownGet,

		// lark_sheet_search_replace
		CellsSearch,
		CellsReplace,

		// lark_sheet_write_cells
		CellsSet,
		CellsSetStyle,
		CellsSetImage,
		CsvPut,
		DropdownSet,

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

		// lark_sheet_batch_update
		BatchUpdate,
		CellsBatchSetStyle,
		DropdownUpdate,
		DropdownDelete,
	}
}
