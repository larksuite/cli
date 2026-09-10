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
		// Two locator decorations: the highest-frequency misspelling of
		// --spreadsheet-token, through the common declarative alias contract,
		// and --local-path for a file opened on this host. Each copies the flag
		// slice before touching it -- shortcut values are package globals and
		// Shortcuts may be called more than once in tests or embedders.
		all[i].Flags = withLocalPathLocator(withSpreadsheetTokenAlias(all[i].Flags))
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

// withLocalPathLocator mounts --local-path beside --spreadsheet-token, so an
// office file opened from this host can be addressed by its path instead of by
// a token the caller would otherwise have to derive itself.
//
// It keys off the presence of --spreadsheet-token rather than a command list:
// the two shortcuts without that flag (+workbook-create, +workbook-import) both
// CREATE a spreadsheet, so neither takes a locator at all, and +workbook-import
// already spells its own local file --file.
//
// The flag is decorated on here rather than declared in data/flag-defs.json for
// the reason localPathFlag records: flag-defs is generated from
// sheet-skill-spec, and the spec rows land in a follow-up.
func withLocalPathLocator(flags []common.Flag) []common.Flag {
	hasFlag := func(name string) bool {
		for i := range flags {
			if flags[i].Name == name {
				return true
			}
		}
		return false
	}
	if !hasFlag("spreadsheet-token") || hasFlag(localPathFlag) {
		return flags
	}
	// Copy before appending: shortcut values are package globals, and appending
	// in place could write into an array another caller still shares.
	decorated := append([]common.Flag(nil), flags...)
	return append(decorated, common.Flag{
		Name: localPathFlag,
		Type: "string",
		// No backquotes in the description: cobra reads a backquoted word as the
		// flag's value placeholder, which is why --url renders its own type as
		// "--spreadsheet-token". Matching that would make the locator group
		// consistently confusing rather than one flag less so.
		Desc: "Path to a local office spreadsheet file on this host, addressed by a token derived from that path (XOR with --url / --spreadsheet-token). Spell the path the same way every time: a relative and an absolute path to one file resolve to two different documents",
	})
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
		CondFormatResultGet,
		FilterList,
		FilterViewList,
		SparklineList,
		FloatImageList,

		// Object CRUD (3 per skill)
		ChartCreate, ChartUpdate, ChartDelete,
		ChartCreateBasic, ChartConfigUpdate, ChartDataUpdate,
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
		BatchChartCreate,
		BatchChartUpdate,
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
