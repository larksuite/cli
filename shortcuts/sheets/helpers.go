// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package sheets contains lark-sheets shortcuts aligned with the
// sheet-skill-spec canonical layout. Each shortcut wraps a single
// sheet-ai-skills tool behind the One-OpenAPI endpoint
// (sheet_ai/v2/.../tools/invoke_{read,write}).
package sheets

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// resolveSpreadsheetToken applies the public --url / --spreadsheet-token XOR
// pair shared by every sheets canonical shortcut and returns the resolved
// token. Network-free, safe to call from Validate and DryRun.
func resolveSpreadsheetToken(runtime *common.RuntimeContext) (string, error) {
	if err := common.ExactlyOne(runtime, "url", "spreadsheet-token"); err != nil {
		return "", err
	}
	if token := strings.TrimSpace(runtime.Str("spreadsheet-token")); token != "" {
		if err := validate.RejectControlChars(token, "spreadsheet-token"); err != nil {
			return "", common.FlagErrorf("%v", err)
		}
		return token, nil
	}

	url := strings.TrimSpace(runtime.Str("url"))
	token := extractSpreadsheetToken(url)
	if token == "" || token == url {
		return "", common.FlagErrorf("--url must be a spreadsheet URL like https://.../sheets/<token>")
	}
	if err := validate.RejectControlChars(token, "url"); err != nil {
		return "", common.FlagErrorf("%v", err)
	}
	return token, nil
}

// extractSpreadsheetToken pulls the token segment out of a /sheets/<token>
// or /spreadsheets/<token> URL. Returns the input unchanged when no known
// prefix is present (callers must check token != originalInput).
func extractSpreadsheetToken(input string) string {
	input = strings.TrimSpace(input)
	for _, prefix := range []string{"/sheets/", "/spreadsheets/"} {
		if idx := strings.Index(input, prefix); idx >= 0 {
			token := input[idx+len(prefix):]
			if idx2 := strings.IndexAny(token, "/?#"); idx2 >= 0 {
				token = token[:idx2]
			}
			return token
		}
	}
	return input
}

// resolveSheetSelector validates the --sheet-id / --sheet-name XOR and
// returns whichever was supplied. Network-free.
//
// Returned tuple: (sheetID, sheetName). Exactly one is non-empty — callers
// pass both through to the tool input; the server picks whichever fits.
func resolveSheetSelector(runtime *common.RuntimeContext) (sheetID, sheetName string, err error) {
	if err := common.ExactlyOne(runtime, "sheet-id", "sheet-name"); err != nil {
		return "", "", err
	}
	if id := strings.TrimSpace(runtime.Str("sheet-id")); id != "" {
		if err := validate.RejectControlChars(id, "sheet-id"); err != nil {
			return "", "", common.FlagErrorf("%v", err)
		}
		return id, "", nil
	}
	name := strings.TrimSpace(runtime.Str("sheet-name"))
	if err := validate.RejectControlChars(name, "sheet-name"); err != nil {
		return "", "", common.FlagErrorf("%v", err)
	}
	return "", name, nil
}

// validateViaInput shrinks a shortcut's Validate to the minimal
// "token + ask the xxxInput builder if everything else is OK" pattern.
// The builder owns the sheet selector and shortcut-specific checks
// (--range required, --start >= 0, ...), so Validate no longer duplicates
// them — the same error fires whether the shortcut runs standalone or as a
// +batch-update sub-op. Use the inline form when the builder needs extra
// arguments (operation enum, withMergeType bool, ...).
func validateViaInput(
	build func(fv flagView, token, sheetID, sheetName string) (map[string]interface{}, error),
) func(ctx context.Context, runtime *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
		sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
		_, err = build(runtime, token, sheetID, sheetName)
		return err
	}
}

// requireSheetSelector is the flagView-agnostic counterpart of
// resolveSheetSelector: given the already-extracted (sheetID, sheetName) pair,
// it enforces the same XOR and control-char rules.
//
// Every batchable xxxInput builder calls this at the top so the same friendly
// error fires whether the shortcut runs standalone (Validate sees the error
// through the builder) or as a +batch-update sub-op (translator sees it
// directly, prefixed by operations[i]). Without this, batch sub-ops
// missing --sheet-id would slip through CLI validation and only fail on the
// server with an opaque "sheet undefined not found".
func requireSheetSelector(sheetID, sheetName string) error {
	sheetID = strings.TrimSpace(sheetID)
	sheetName = strings.TrimSpace(sheetName)
	if sheetID == "" && sheetName == "" {
		return common.FlagErrorf("specify at least one of --sheet-id or --sheet-name")
	}
	if sheetID != "" && sheetName != "" {
		return common.FlagErrorf("--sheet-id and --sheet-name are mutually exclusive")
	}
	if sheetID != "" {
		if err := validate.RejectControlChars(sheetID, "sheet-id"); err != nil {
			return common.FlagErrorf("%v", err)
		}
	} else {
		if err := validate.RejectControlChars(sheetName, "sheet-name"); err != nil {
			return common.FlagErrorf("%v", err)
		}
	}
	return nil
}

// optionalSheetSelector is the "at most one" counterpart of
// requireSheetSelector: both empty is acceptable (the backend tool then
// decides what to do — e.g. manage_pivot_table_object auto-creates a new
// sub-sheet to host the pivot), and both set is rejected. Control-char
// validation still applies whenever a value is provided.
//
// Used by shortcuts whose backend tool treats sheet_id/sheet_name as the
// placement target rather than the operation context (currently only
// +pivot-create). Other shortcuts continue to use requireSheetSelector.
//
// idFlagName / nameFlagName parameterize the flag names quoted back in
// the mutex / control-char errors — +pivot-create exposes the placement
// selector as `--target-sheet-id` / `--target-sheet-name`, not the
// generic `--sheet-id` / `--sheet-name`, and the error wording must
// match what the user actually typed.
func optionalSheetSelector(sheetID, sheetName, idFlagName, nameFlagName string) error {
	sheetID = strings.TrimSpace(sheetID)
	sheetName = strings.TrimSpace(sheetName)
	if sheetID != "" && sheetName != "" {
		return common.FlagErrorf("--%s and --%s are mutually exclusive", idFlagName, nameFlagName)
	}
	if sheetID != "" {
		if err := validate.RejectControlChars(sheetID, idFlagName); err != nil {
			return common.FlagErrorf("%v", err)
		}
	} else if sheetName != "" {
		if err := validate.RejectControlChars(sheetName, nameFlagName); err != nil {
			return common.FlagErrorf("%v", err)
		}
	}
	return nil
}

// sheetSelectorForToolInput packs --sheet-id / --sheet-name into the tool
// input map, omitting empty fields. Use after resolveSheetSelector returns.
func sheetSelectorForToolInput(input map[string]interface{}, sheetID, sheetName string) {
	if sheetID != "" {
		input["sheet_id"] = sheetID
	}
	if sheetName != "" {
		input["sheet_name"] = sheetName
	}
}

// sheetSelectorPlaceholder returns a human-readable identifier for the
// selected sheet, suitable for DryRun output. Avoids leaking that --sheet-name
// would be resolved server-side at execute time.
func sheetSelectorPlaceholder(sheetID, sheetName string) string {
	if sheetID != "" {
		return sheetID
	}
	return "<resolve:" + sheetName + ">"
}

// parseJSONFlag parses a JSON string from a flag value. Returns nil when the
// flag is empty (caller decides if that's acceptable). Used by --data /
// --style / --options / --ranges / --colors and friends.
func parseJSONFlag(runtime flagView, name string) (interface{}, error) {
	raw := strings.TrimSpace(runtime.Str(name))
	if raw == "" {
		return nil, nil
	}
	var out interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, common.FlagErrorf("--%s: invalid JSON: %v", name, err)
	}
	// Schema-driven flag validation at the user-input boundary. Skips
	// --properties (validated at the input-builder tail after enhance
	// hooks fill in flat-flag-derived fields) and any flag without an
	// embedded schema entry.
	if err := validateParsedJSONFlag(runtime, name, out); err != nil {
		return nil, err
	}
	return out, nil
}

// requireJSONObject is parseJSONFlag + a type assertion to map[string]interface{}.
func requireJSONObject(runtime flagView, name string) (map[string]interface{}, error) {
	v, err := parseJSONFlag(runtime, name)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, common.FlagErrorf("--%s is required", name)
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, common.FlagErrorf("--%s must be a JSON object", name)
	}
	return m, nil
}

// requireJSONArray is parseJSONFlag + a type assertion to []interface{}.
func requireJSONArray(runtime flagView, name string) ([]interface{}, error) {
	v, err := parseJSONFlag(runtime, name)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, common.FlagErrorf("--%s is required", name)
	}
	a, ok := v.([]interface{})
	if !ok {
		return nil, common.FlagErrorf("--%s must be a JSON array", name)
	}
	return a, nil
}

// ─── style flags (shared by +cells-set-style and +cells-batch-set-style) ─

// buildCellStyleFromFlags reads the 11 flat style flags and returns the
// cell_styles map expected by set_cell_range. Skips any flag the user
// didn't set so partial styles work.
func buildCellStyleFromFlags(runtime flagView) map[string]interface{} {
	style := map[string]interface{}{}
	if v := runtime.Str("background-color"); v != "" {
		style["background_color"] = v
	}
	if v := runtime.Str("font-color"); v != "" {
		style["font_color"] = v
	}
	if runtime.Changed("font-size") && runtime.Float64("font-size") > 0 {
		style["font_size"] = runtime.Float64("font-size")
	}
	if v := runtime.Str("font-style"); v != "" {
		style["font_style"] = v
	}
	if v := runtime.Str("font-weight"); v != "" {
		style["font_weight"] = v
	}
	if v := runtime.Str("font-line"); v != "" {
		style["font_line"] = v
	}
	if v := runtime.Str("horizontal-alignment"); v != "" {
		style["horizontal_alignment"] = v
	}
	if v := runtime.Str("vertical-alignment"); v != "" {
		style["vertical_alignment"] = v
	}
	if v := runtime.Str("word-wrap"); v != "" {
		style["word_wrap"] = v
	}
	if v := runtime.Str("number-format"); v != "" {
		style["number_format"] = v
	}
	return style
}

// borderStylesFromFlag parses --border-styles as a JSON object (top/bottom/
// left/right with style sub-objects). Returns nil when the flag is empty.
func borderStylesFromFlag(runtime flagView) (map[string]interface{}, error) {
	if runtime.Str("border-styles") == "" {
		return nil, nil
	}
	v, err := parseJSONFlag(runtime, "border-styles")
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, common.FlagErrorf("--border-styles must be a JSON object")
	}
	return m, nil
}

// requireAnyStyleFlag ensures at least one style-defining flag (style or
// border) is set — otherwise the request would do nothing.
func requireAnyStyleFlag(runtime flagView) error {
	if len(buildCellStyleFromFlags(runtime)) > 0 {
		return nil
	}
	if runtime.Str("border-styles") != "" {
		return nil
	}
	return common.FlagErrorf("at least one style flag is required (e.g. --background-color, --font-weight, --border-styles)")
}
