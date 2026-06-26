// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
		// Accept --token as a parse-time alias for --spreadsheet-token (the
		// single highest-frequency reflex misspelling in eval traces) on every
		// shortcut that registers --spreadsheet-token, so the typo costs zero
		// round-trips instead of an unknown-flag failure. Wired through the
		// existing PostMount hook and composed onto any prior PostMount, so the
		// common framework needs no change at all.
		if hasFlag(all[i].Flags, "spreadsheet-token") {
			all[i].PostMount = withTokenAlias(all[i].PostMount)
		}
	}
	return all
}

func hasFlag(flags []common.Flag, name string) bool {
	for _, fl := range flags {
		if fl.Name == name {
			return true
		}
	}
	return false
}

// withTokenAlias wraps an optional PostMount so that, after it runs, --token
// resolves to --spreadsheet-token at parse time via pflag's normalize hook (no
// duplicate flag in --help). It preserves any pre-existing PostMount — e.g.
// +csv-put's --range / --start-cell flag-group setup — by running it first.
func withTokenAlias(prev func(cmd *cobra.Command)) func(cmd *cobra.Command) {
	return func(cmd *cobra.Command) {
		if prev != nil {
			prev(cmd)
		}
		cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
			if name == "token" {
				return pflag.NormalizedName("spreadsheet-token")
			}
			return pflag.NormalizedName(name)
		})
	}
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

		// lark_sheet_read_data
		CellsGet,
		CsvGet,
		DropdownGet,
		TableGet,

		// lark_sheet_search_replace
		CellsSearch,
		CellsReplace,

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

		// lark_sheet_batch_update
		BatchUpdate,
		CellsBatchSetStyle,
		CellsBatchClear,
		DropdownUpdate,
		DropdownDelete,
	}
}
