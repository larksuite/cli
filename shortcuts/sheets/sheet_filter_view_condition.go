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

func filterViewConditionBasePath(token, sheetID, filterViewID string) string {
	return fmt.Sprintf("%s/conditions", filterViewItemPath(token, sheetID, filterViewID))
}

func filterViewConditionItemPath(token, sheetID, filterViewID, conditionID string) string {
	return fmt.Sprintf("%s/%s", filterViewConditionBasePath(token, sheetID, filterViewID), validate.EncodePathSegment(conditionID))
}

var SheetCreateFilterViewCondition = common.Shortcut{
	Service:     "sheets",
	Command:     "+create-filter-view-condition",
	Description: "Create a filter condition on a filter view",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL (required if --spreadsheet-token is not set)"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token (required if --url is not set)"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "filter-view-id", Desc: "filter view ID", Required: true},
		{Name: "condition-id", Desc: "column letter (e.g. E)", Required: true},
		{Name: "filter-type", Desc: "filter type: hiddenValue, number, text, color", Required: true},
		{Name: "compare-type", Desc: "comparison operator (e.g. less, beginsWith, between)"},
		{Name: "expected", Desc: "filter values JSON array (e.g. [\"6\"])", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateFilterViewToken(runtime); err != nil {
			return err
		}
		return validateExpectedFlag(runtime.Str("expected"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFilterViewToken(runtime)
		body := buildConditionBody(runtime, true)
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions").
			Body(body).Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).Set("filter_view_id", runtime.Str("filter-view-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		body := buildConditionBody(runtime, true)
		data, err := runtime.CallAPI("POST", filterViewConditionBasePath(token, runtime.Str("sheet-id"), runtime.Str("filter-view-id")), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetUpdateFilterViewCondition = common.Shortcut{
	Service:     "sheets",
	Command:     "+update-filter-view-condition",
	Description: "Update a filter condition on a filter view",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL (required if --spreadsheet-token is not set)"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token (required if --url is not set)"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "filter-view-id", Desc: "filter view ID", Required: true},
		{Name: "condition-id", Desc: "column letter (e.g. E)", Required: true},
		{Name: "filter-type", Desc: "filter type: hiddenValue, number, text, color"},
		{Name: "compare-type", Desc: "comparison operator"},
		{Name: "expected", Desc: "filter values JSON array"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateFilterViewToken(runtime); err != nil {
			return err
		}
		if !runtime.Cmd.Flags().Changed("filter-type") &&
			!runtime.Cmd.Flags().Changed("compare-type") &&
			!runtime.Cmd.Flags().Changed("expected") {
			return common.FlagErrorf("specify at least one of --filter-type, --compare-type, or --expected")
		}
		if s := runtime.Str("expected"); s != "" {
			return validateExpectedFlag(s)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFilterViewToken(runtime)
		body := buildConditionBody(runtime, false)
		return common.NewDryRunAPI().
			PUT("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions/:condition_id").
			Body(body).Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).
			Set("filter_view_id", runtime.Str("filter-view-id")).Set("condition_id", runtime.Str("condition-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		body := buildConditionBody(runtime, false)
		data, err := runtime.CallAPI("PUT",
			filterViewConditionItemPath(token, runtime.Str("sheet-id"), runtime.Str("filter-view-id"), runtime.Str("condition-id")),
			nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetListFilterViewConditions = common.Shortcut{
	Service:     "sheets",
	Command:     "+list-filter-view-conditions",
	Description: "List all filter conditions of a filter view",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL (required if --spreadsheet-token is not set)"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token (required if --url is not set)"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "filter-view-id", Desc: "filter view ID", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := validateFilterViewToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFilterViewToken(runtime)
		return common.NewDryRunAPI().
			GET("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions/query").
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).Set("filter_view_id", runtime.Str("filter-view-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		data, err := runtime.CallAPI("GET",
			filterViewConditionBasePath(token, runtime.Str("sheet-id"), runtime.Str("filter-view-id"))+"/query",
			nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetGetFilterViewCondition = common.Shortcut{
	Service:     "sheets",
	Command:     "+get-filter-view-condition",
	Description: "Get a filter condition by column",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL (required if --spreadsheet-token is not set)"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token (required if --url is not set)"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "filter-view-id", Desc: "filter view ID", Required: true},
		{Name: "condition-id", Desc: "column letter (e.g. E)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := validateFilterViewToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFilterViewToken(runtime)
		return common.NewDryRunAPI().
			GET("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions/:condition_id").
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).
			Set("filter_view_id", runtime.Str("filter-view-id")).Set("condition_id", runtime.Str("condition-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		data, err := runtime.CallAPI("GET",
			filterViewConditionItemPath(token, runtime.Str("sheet-id"), runtime.Str("filter-view-id"), runtime.Str("condition-id")),
			nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetDeleteFilterViewCondition = common.Shortcut{
	Service:     "sheets",
	Command:     "+delete-filter-view-condition",
	Description: "Delete a filter condition from a filter view",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL (required if --spreadsheet-token is not set)"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token (required if --url is not set)"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "filter-view-id", Desc: "filter view ID", Required: true},
		{Name: "condition-id", Desc: "column letter (e.g. E)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := validateFilterViewToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFilterViewToken(runtime)
		return common.NewDryRunAPI().
			DELETE("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id/conditions/:condition_id").
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).
			Set("filter_view_id", runtime.Str("filter-view-id")).Set("condition_id", runtime.Str("condition-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		data, err := runtime.CallAPI("DELETE",
			filterViewConditionItemPath(token, runtime.Str("sheet-id"), runtime.Str("filter-view-id"), runtime.Str("condition-id")),
			nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// validateExpectedFlag checks that --expected is a valid JSON array.
func validateExpectedFlag(s string) error {
	if s == "" {
		return nil
	}
	var arr []interface{}
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return fmt.Errorf("--expected must be a JSON array (e.g. [\"6\"]), got: %s", s)
	}
	return nil
}

// buildConditionBody constructs the request body for condition create/update.
func buildConditionBody(runtime *common.RuntimeContext, includeConditionID bool) map[string]interface{} {
	body := map[string]interface{}{}
	if includeConditionID {
		body["condition_id"] = runtime.Str("condition-id")
	}
	if s := runtime.Str("filter-type"); s != "" {
		body["filter_type"] = s
	}
	if s := runtime.Str("compare-type"); s != "" {
		body["compare_type"] = s
	}
	if s := runtime.Str("expected"); s != "" {
		var arr []interface{}
		// Validate already ensures this is a valid JSON array.
		_ = json.Unmarshal([]byte(s), &arr)
		body["expected"] = arr
	}
	return body
}
