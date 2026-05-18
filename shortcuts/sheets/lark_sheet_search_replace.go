// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_search_replace ────────────────────────────────────────
//
// Wraps search_data (read) and replace_data (write). Both tools take an
// `options` sub-object; the CLI flattens its common booleans
// (--match-case / --match-entire-cell / --regex / --include-formulas) into
// independent flags per the铁律.

// CellsSearch wraps search_data: find cell coordinates matching --find,
// with optional case / regex / whole-cell / formula-text controls.
var CellsSearch = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-search",
	Description: "Find cells matching --find in a spreadsheet (case / regex / whole-cell / formula-text controls).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "find", Required: true, Desc: "text to search for (regex when --regex is set)"},
		common.Flag{Name: "range", Desc: "optional A1 range to scope the search"},
		common.Flag{Name: "match-case", Type: "bool", Desc: "case-sensitive match"},
		common.Flag{Name: "match-entire-cell", Type: "bool", Desc: "match the entire cell content only"},
		common.Flag{Name: "regex", Type: "bool", Desc: "treat --find as a regular expression"},
		common.Flag{Name: "include-formulas", Type: "bool", Desc: "also search inside formula text"},
		common.Flag{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset (use next_offset from previous page)"},
		common.Flag{Name: "max-matches", Type: "int", Default: "5000", Hidden: true, Desc: "anti-burst match cap"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("find")) == "" {
			return common.FlagErrorf("--find is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindRead, "search_data", searchInput(runtime, token, sheetID, sheetName))
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
		out, err := callTool(ctx, runtime, token, ToolKindRead, "search_data", searchInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func searchInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{
		"excel_id":    token,
		"search_term": runtime.Str("find"),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if r := strings.TrimSpace(runtime.Str("range")); r != "" {
		input["range"] = r
	}
	if runtime.Changed("offset") && runtime.Int("offset") > 0 {
		input["offset"] = runtime.Int("offset")
	}
	if opts := searchReplaceOptions(runtime); len(opts) > 0 {
		input["options"] = opts
	}
	if n := runtime.Int("max-matches"); n > 0 {
		input["max_matches"] = n
	}
	return input
}

// searchReplaceOptions packs the four shared boolean flags into the tool's
// `options` sub-object. Empty result → caller should omit the field.
func searchReplaceOptions(runtime *common.RuntimeContext) map[string]interface{} {
	opts := map[string]interface{}{}
	if runtime.Bool("match-case") {
		opts["match_case"] = true
	}
	if runtime.Bool("match-entire-cell") {
		opts["match_entire_cell"] = true
	}
	if runtime.Bool("regex") {
		opts["regex"] = true
	}
	if runtime.Bool("include-formulas") {
		opts["include_formulas"] = true
	}
	return opts
}

// CellsReplace wraps replace_data: find and replace text across a
// spreadsheet, with the same option controls as +cells-search.
var CellsReplace = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-replace",
	Description: "Find and replace text in a spreadsheet (case / regex / whole-cell / formula-text controls).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "find", Required: true, Desc: "text to find (regex when --regex is set)"},
		common.Flag{Name: "replacement", Required: true, Desc: "replacement text (empty string deletes the match)"},
		common.Flag{Name: "range", Desc: "optional A1 range to scope the replace"},
		common.Flag{Name: "match-case", Type: "bool", Desc: "case-sensitive match"},
		common.Flag{Name: "match-entire-cell", Type: "bool", Desc: "match the entire cell content only"},
		common.Flag{Name: "regex", Type: "bool", Desc: "treat --find as a regular expression"},
		common.Flag{Name: "include-formulas", Type: "bool", Desc: "also replace inside formula text"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("find")) == "" {
			return common.FlagErrorf("--find is required")
		}
		if !runtime.Changed("replacement") {
			return common.FlagErrorf("--replacement is required (pass an empty string to delete matches)")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "replace_data", replaceInput(runtime, token, sheetID, sheetName))
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
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "replace_data", replaceInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Always preview with --dry-run before running — replace can mutate every matching cell across the sheet.",
	},
}

func replaceInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{
		"excel_id":     token,
		"search_term":  runtime.Str("find"),
		"replace_term": runtime.Str("replacement"),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if r := strings.TrimSpace(runtime.Str("range")); r != "" {
		input["range"] = r
	}
	if opts := searchReplaceOptions(runtime); len(opts) > 0 {
		input["options"] = opts
	}
	return input
}
