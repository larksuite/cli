// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// Cell stamps may omit the per-sheet wrapper only when the invocation names
// one target. The existing style parser still checks every field and budget.
func stylesPutItems(runtime flagView, resolvedName string) ([]map[string]interface{}, error) {
	id, name := strings.TrimSpace(runtime.Str("sheet-id")), strings.TrimSpace(runtime.Str("sheet-name"))
	if err := optionalSheetSelector(id, name, "sheet-id", "sheet-name"); err != nil {
		return nil, err
	}
	v, err := parseJSONFlag(runtime, "styles")
	if err != nil {
		return nil, err
	}
	stamps := []map[string]interface{}(nil)
	sectioned := func(item map[string]interface{}) bool {
		for key := range item {
			for _, section := range workbookCreateStyleItemKeys {
				if key == section && section != "name" {
					return true
				}
			}
			if (styleItemKeyAliases[key] != "" && styleItemKeyAliases[key] != "name") || key == "styles" {
				return true
			}
		}
		return false
	}
	switch shape := v.(type) {
	case map[string]interface{}:
		_, hasRange := shape["range"]
		if !sectioned(shape) && (runtime.Changed("range") || hasRange) {
			stamps = []map[string]interface{}{shape}
		}
	case []interface{}:
		if len(shape) > 0 {
			stamps = make([]map[string]interface{}, 0, len(shape))
			for _, raw := range shape {
				item, ok := raw.(map[string]interface{})
				_, hasRange := item["range"]
				if !ok || sectioned(item) || !hasRange {
					stamps = nil
					break
				}
				stamps = append(stamps, item)
			}
		}
	}
	if stamps != nil {
		if runtime.Changed("range") {
			if len(stamps) != 1 {
				return nil, sheetsValidationForFlag("range", "--range cannot target multiple style stamps")
			}
			stamp := stamps[0]
			if given, ok := stamp["range"]; ok && given != runtime.Str("range") {
				return nil, sheetsValidationForFlag("range", "--range conflicts with the style stamp range")
			}
			stamp["range"] = runtime.Str("range")
		}
		groups := map[string][]interface{}{}
		order := make([]string, 0, len(stamps))
		for _, stamp := range stamps {
			foldStyleItemKeys(stamp)
			target := name
			if given, present := stamp["name"]; present {
				text, ok := given.(string)
				if !ok || strings.TrimSpace(text) == "" {
					return nil, sheetsValidationForFlag("styles", "a stamp name must be a non-empty string")
				}
				if name != "" && name != text {
					return nil, sheetsValidationForFlag("sheet-name", "--sheet-name conflicts with a stamp name")
				}
				target = text
				delete(stamp, "name")
			}
			if target == "" && id == "" {
				return nil, sheetsValidationForFlag("styles", "cell-style stamps require an explicit name or --sheet-id / --sheet-name")
			}
			if _, present := groups[target]; !present {
				order = append(order, target)
			}
			groups[target] = append(groups[target], stamp)
		}
		items := make([]interface{}, 0, len(groups))
		for _, target := range order {
			item := map[string]interface{}{"cell_styles": groups[target]}
			if target != "" {
				item["name"] = target
			}
			items = append(items, item)
		}
		v = items
	} else if runtime.Changed("range") {
		return nil, sheetsValidationForFlag("range", "--range requires a single flat style object; sectioned specs carry their own ranges")
	}
	items, err := parseWorkbookCreateStylesItems(v, stylesFromFlag)
	if err != nil {
		return nil, err
	}
	if id != "" && len(items) != 1 {
		return nil, sheetsValidationForFlag("sheet-id", "--sheet-id requires exactly one styles item")
	}
	if len(items) == 1 {
		if given, present := items[0]["name"]; present {
			if name != "" && given != name {
				return nil, sheetsValidationForFlag("sheet-name", "--sheet-name conflicts with the styles item name")
			}
			if resolvedName != "" && given != resolvedName {
				return nil, sheetsValidationForFlag("sheet-id", "--sheet-id names %q but the styles item names %v", resolvedName, given)
			}
		} else if resolvedName != "" {
			items[0]["name"] = resolvedName
		} else if name != "" {
			items[0]["name"] = name
		}
	}
	return items, nil
}

// Resolve an explicit ID once before any write. The same read supplies grid
// bounds, and the live name validates both item names and range qualifiers.
func stylesPutTarget(ctx context.Context, runtime *common.RuntimeContext, token string) (string, sheetRangeBounder, error) {
	id := strings.TrimSpace(runtime.Str("sheet-id"))
	if id == "" {
		return "", newSheetGridBounder(ctx, runtime, token), nil
	}
	byName, dims, err := listSheetIDsByName(ctx, runtime, token)
	if err != nil {
		return "", nil, err
	}
	for name, found := range byName {
		if found != id {
			continue
		}
		grid := sheetGrid{rows: dims[name].rows, cols: dims[name].cols}
		return name, func(_ string, rng string) (string, bool, error) {
			fitted, ok := boundRangeToGrid(rng, grid)
			return fitted, ok, nil
		}, nil
	}
	return "", nil, sheetsValidationForFlag("sheet-id", "--sheet-id %q was not found in the workbook", id)
}
