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
		all[i].Flags = withCommandFlagAliases(all[i].Command, all[i].Flags)
		// +chart-create grows --print-example (minimal per-type --properties
		// templates) — the biggest --print-schema consumer in eval traces.
		if all[i].Command == "+chart-create" {
			all[i].PostMount = withChartPrintExample(all[i].PostMount)
		}
		// Sheets-scoped flag ergonomics (unknown-flag hints with the valid
		// flags inlined, enum vocabulary normalization) ride the existing
		// PostMount composition, so no other domain's behavior shifts.
		all[i].PostMount = withFlagErgonomics(all[i].PostMount)
		all[i] = withOmittedSelectorReadScope(all[i])
	}
	return all
}

// withOmittedSelectorReadScope declares the read scope the omitted-selector
// path needs. resolveOmittedSheetSelector answers a missing --sheet-name by
// asking get_workbook_structure, which is a READ on commands that otherwise
// declare only the write scope -- so a least-privilege token passes pre-flight
// and then fails inside the lookup.
//
// Derived from the shortcut's own flags rather than written out per command:
// the set is exactly "has a sheet selector to omit", and a hand-kept list
// would drift the moment a command gains or loses one. Conditional, because
// only the calls that actually omit the selector ever perform the read.
func withOmittedSelectorReadScope(sc common.Shortcut) common.Shortcut {
	if selectorMustBeExplicit[sc.Command] || !hasSheetSelectorFlag(sc.Flags) {
		return sc
	}
	for _, existing := range append(sc.DeclaredScopesForIdentity("user"), sc.DeclaredScopesForIdentity("bot")...) {
		if existing == sheetsStructureReadScope {
			return sc
		}
	}
	sc.ConditionalScopes = append(append([]string(nil), sc.ConditionalScopes...), sheetsStructureReadScope)
	return sc
}

func hasSheetSelectorFlag(flags []common.Flag) bool {
	for _, f := range flags {
		if f.Name == "sheet-name" || f.Name == "sheet-id" {
			return true
		}
	}
	return false
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

// Exact synonyms use the framework's declared alias contract. These observed
// spellings keep the canonical flag's value grammar, defaults and validation.
var canonicalCommandFlagAliases = map[string]map[string]string{
	"+cols-resize":           {"col-range": "range", "col": "range", "column": "range", "sizes": "widths", "columns": "widths", "pixel": "width", "col-start": "start-col", "col-end": "end-col", "start-column": "start-col", "end-column": "end-col"},
	"+rows-resize":           {"row-range": "range", "row": "range", "sizes": "heights", "pixel": "height"},
	"+sheet-copy":            {"new-sheet-name": "title", "new-title": "title"},
	"+chart-create-basic":    {"anchor": "anchor-cell", "position": "anchor-cell"},
	"+dim-delete":            {"end": "end-index"},
	"+styles-put":            {"spec": "styles", "style": "styles"},
	"+csv-get":               {"output-file": "output-path"},
	"+cells-clear":           {"clear-type": "scope"},
	"+cells-batch-clear":     {"range": "ranges"},
	"+cells-batch-set-style": {"range": "ranges"},
	"+dropdown-update":       {"range": "ranges"},
	"+dropdown-delete":       {"range": "ranges"},
}

var canonicalSheetFlagAliases = map[string]string{"worksheet-id": "sheet-id", "worksheet-name": "sheet-name"}

func withCommandFlagAliases(command string, flags []common.Flag) []common.Flag {
	aliases := make(map[string]string, len(canonicalCommandFlagAliases[command])+len(canonicalSheetFlagAliases))
	for alias, target := range canonicalCommandFlagAliases[command] {
		aliases[alias] = target
	}
	for alias, target := range canonicalSheetFlagAliases {
		aliases[alias] = target
	}
	decorated := append([]common.Flag(nil), flags...)
	for i := range decorated {
		for _, alias := range sortedKeys(aliases) {
			if aliases[alias] == decorated[i].Name {
				decorated[i].Aliases = append(append([]string(nil), decorated[i].Aliases...), alias)
			}
		}
	}
	return decorated
}

// Batch inputs share both the declared synonyms and legacy value adapters.
func commandInputAliases(command string) map[string]string {
	aliases := make(map[string]string, len(commandFlagAliases[command])+len(canonicalCommandFlagAliases[command]))
	for alias, target := range commandFlagAliases[command] {
		aliases[alias] = target
	}
	for alias, target := range canonicalCommandFlagAliases[command] {
		aliases[alias] = target
	}
	for alias, target := range canonicalSheetFlagAliases {
		for _, flag := range flagDefs[command].Flags {
			if flag.Name == target {
				aliases[alias] = target
				break
			}
		}
	}
	return aliases
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
