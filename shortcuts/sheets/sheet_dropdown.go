// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func dataValidationBasePath(token string) string {
	return fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dataValidation",
		validate.EncodePathSegment(token))
}

func dataValidationSheetPath(token, sheetID string) string {
	return fmt.Sprintf("%s/%s", dataValidationBasePath(token), validate.EncodePathSegment(sheetID))
}

func validateDropdownToken(runtime *common.RuntimeContext) (string, error) {
	token := runtime.Str("spreadsheet-token")
	if runtime.Str("url") != "" {
		token = extractSpreadsheetToken(runtime.Str("url"))
	}
	if token == "" {
		return "", common.FlagErrorf("specify --url or --spreadsheet-token")
	}
	return token, nil
}

func parseJSONStringArray(flagName, value string) ([]interface{}, error) {
	var typed []string
	if err := json.Unmarshal([]byte(value), &typed); err != nil {
		return nil, common.FlagErrorf("--%s must be a JSON array of strings: %v", flagName, err)
	}
	if typed == nil {
		return nil, common.FlagErrorf("--%s must be a JSON array, got null", flagName)
	}
	arr := make([]interface{}, len(typed))
	for i, s := range typed {
		arr[i] = s
	}
	return arr, nil
}

func validateRangesFlag(runtime *common.RuntimeContext) ([]interface{}, error) {
	ranges, err := parseJSONStringArray("ranges", runtime.Str("ranges"))
	if err != nil {
		return nil, err
	}
	if len(ranges) == 0 {
		return nil, common.FlagErrorf("--ranges must not be empty")
	}
	for i, r := range ranges {
		s, _ := r.(string)
		if _, _, ok := splitSheetRange(s); !ok {
			return nil, common.FlagErrorf("--ranges[%d] %q must be a fully qualified range with sheet ID prefix (e.g. <sheetId>!A2:A100)", i, s)
		}
	}
	return ranges, nil
}

func buildDropdownBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	condValues, err := parseJSONStringArray("condition-values", runtime.Str("condition-values"))
	if err != nil {
		return nil, err
	}
	if len(condValues) == 0 {
		return nil, common.FlagErrorf("--condition-values must not be empty")
	}

	dv := map[string]interface{}{
		"conditionValues": condValues,
	}

	opts := map[string]interface{}{}
	if runtime.Cmd.Flags().Changed("multiple") {
		opts["multipleValues"] = runtime.Bool("multiple")
	}
	if runtime.Cmd.Flags().Changed("highlight") {
		opts["highlightValidData"] = runtime.Bool("highlight")
	}
	if runtime.Str("colors") != "" {
		colors, err := parseJSONStringArray("colors", runtime.Str("colors"))
		if err != nil {
			return nil, err
		}
		if len(colors) != len(condValues) {
			return nil, common.FlagErrorf("--colors length (%d) must match --condition-values length (%d)", len(colors), len(condValues))
		}
		opts["colors"] = colors
	}
	if len(opts) > 0 {
		dv["options"] = opts
	}

	return dv, nil
}

// SheetSetDropdown sets dropdown list validation on a range.
var SheetSetDropdown = common.Shortcut{
	Service:     "sheets",
	Command:     "+set-dropdown",
	Description: "Set dropdown list on a cell range",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "range", Desc: "cell range (<sheetId>!A2:A100)", Required: true},
		{Name: "condition-values", Desc: `dropdown options as JSON array (e.g. '["opt1","opt2"]'), max 500, each <=100 chars, no commas`, Required: true},
		{Name: "multiple", Desc: "enable multi-select (default false)", Type: "bool"},
		{Name: "highlight", Desc: "color-code options (default false)", Type: "bool"},
		{Name: "colors", Desc: `RGB hex color array (e.g. '["#1FB6C1","#F006C2"]'), must match condition-values length`},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateDropdownToken(runtime); err != nil {
			return err
		}
		if _, _, ok := splitSheetRange(runtime.Str("range")); !ok {
			return common.FlagErrorf("--range must be a fully qualified range with sheet ID prefix (e.g. <sheetId>!A2:A100)")
		}
		_, err := buildDropdownBody(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateDropdownToken(runtime)
		dv, _ := buildDropdownBody(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v2/spreadsheets/:token/dataValidation").
			Body(map[string]interface{}{
				"range":              runtime.Str("range"),
				"dataValidationType": "list",
				"dataValidation":     dv,
			}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateDropdownToken(runtime)
		dv, err := buildDropdownBody(runtime)
		if err != nil {
			return err
		}

		data, err := runtime.CallAPI("POST", dataValidationBasePath(token), nil,
			map[string]interface{}{
				"range":              runtime.Str("range"),
				"dataValidationType": "list",
				"dataValidation":     dv,
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// SheetUpdateDropdown updates dropdown list settings for given ranges.
var SheetUpdateDropdown = common.Shortcut{
	Service:     "sheets",
	Command:     "+update-dropdown",
	Description: "Update dropdown list settings",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "ranges", Desc: `ranges as JSON array (e.g. '["sheetId!A1:A100"]')`, Required: true},
		{Name: "condition-values", Desc: `dropdown options as JSON array (e.g. '["opt1","opt2"]')`, Required: true},
		{Name: "multiple", Desc: "enable multi-select (default false)", Type: "bool"},
		{Name: "highlight", Desc: "color-code options (default false)", Type: "bool"},
		{Name: "colors", Desc: `RGB hex color array, must match condition-values length`},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateDropdownToken(runtime); err != nil {
			return err
		}
		if _, err := validateRangesFlag(runtime); err != nil {
			return err
		}
		_, err := buildDropdownBody(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateDropdownToken(runtime)
		ranges, _ := parseJSONStringArray("ranges", runtime.Str("ranges"))
		dv, _ := buildDropdownBody(runtime)
		return common.NewDryRunAPI().
			PUT("/open-apis/sheets/v2/spreadsheets/:token/dataValidation/:sheet_id").
			Body(map[string]interface{}{
				"ranges":             ranges,
				"dataValidationType": "list",
				"dataValidation":     dv,
			}).
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateDropdownToken(runtime)
		ranges, err := parseJSONStringArray("ranges", runtime.Str("ranges"))
		if err != nil {
			return err
		}
		dv, err := buildDropdownBody(runtime)
		if err != nil {
			return err
		}

		data, err := runtime.CallAPI("PUT", dataValidationSheetPath(token, runtime.Str("sheet-id")), nil,
			map[string]interface{}{
				"ranges":             ranges,
				"dataValidationType": "list",
				"dataValidation":     dv,
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// SheetGetDropdown queries dropdown list settings for a range.
var SheetGetDropdown = common.Shortcut{
	Service:     "sheets",
	Command:     "+get-dropdown",
	Description: "Get dropdown list settings for a range",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "range", Desc: "cell range (<sheetId>!A2:A100)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateDropdownToken(runtime); err != nil {
			return err
		}
		if _, _, ok := splitSheetRange(runtime.Str("range")); !ok {
			return common.FlagErrorf("--range must be a fully qualified range with sheet ID prefix (e.g. <sheetId>!A2:A100)")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateDropdownToken(runtime)
		return common.NewDryRunAPI().
			GET("/open-apis/sheets/v2/spreadsheets/:token/dataValidation?range=:range&dataValidationType=list").
			Set("token", token).Set("range", runtime.Str("range"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateDropdownToken(runtime)
		data, err := runtime.CallAPI("GET", dataValidationBasePath(token),
			map[string]interface{}{
				"range":              runtime.Str("range"),
				"dataValidationType": "list",
			}, nil,
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// SheetDeleteDropdown deletes dropdown list settings from given ranges.
var SheetDeleteDropdown = common.Shortcut{
	Service:     "sheets",
	Command:     "+delete-dropdown",
	Description: "Delete dropdown list from cell ranges",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token"},
		{Name: "ranges", Desc: `ranges as JSON array (e.g. '["sheetId!A2:A100"]'), max 100 ranges`, Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateDropdownToken(runtime); err != nil {
			return err
		}
		_, err := validateRangesFlag(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateDropdownToken(runtime)
		ranges, _ := parseJSONStringArray("ranges", runtime.Str("ranges"))
		dvRanges := make([]interface{}, 0, len(ranges))
		for _, r := range ranges {
			dvRanges = append(dvRanges, map[string]interface{}{"range": r})
		}
		return common.NewDryRunAPI().
			DELETE("/open-apis/sheets/v2/spreadsheets/:token/dataValidation").
			Body(map[string]interface{}{
				"dataValidationRanges": dvRanges,
			}).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateDropdownToken(runtime)
		ranges, err := parseJSONStringArray("ranges", runtime.Str("ranges"))
		if err != nil {
			return err
		}

		dvRanges := make([]interface{}, 0, len(ranges))
		for _, r := range ranges {
			dvRanges = append(dvRanges, map[string]interface{}{"range": r})
		}

		data, err := runtime.CallAPI("DELETE", dataValidationBasePath(token), nil,
			map[string]interface{}{
				"dataValidationRanges": dvRanges,
			},
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
