// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func filterViewBasePath(token, sheetID string) string {
	return fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/sheets/%s/filter_views",
		validate.EncodePathSegment(token), validate.EncodePathSegment(sheetID))
}

func filterViewItemPath(token, sheetID, filterViewID string) string {
	return fmt.Sprintf("%s/%s", filterViewBasePath(token, sheetID), validate.EncodePathSegment(filterViewID))
}

func filterViewConditionBasePath(token, sheetID, filterViewID string) string {
	return fmt.Sprintf("%s/conditions", filterViewItemPath(token, sheetID, filterViewID))
}

func filterViewConditionItemPath(token, sheetID, filterViewID, conditionID string) string {
	return fmt.Sprintf("%s/%s", filterViewConditionBasePath(token, sheetID, filterViewID), validate.EncodePathSegment(conditionID))
}

func validateFilterViewToken(runtime *common.RuntimeContext) (string, error) {
	return validateSheetManageToken(runtime)
}

func hasNonEmptyStringFlag(runtime *common.RuntimeContext, name string) bool {
	return runtime.Cmd.Flags().Changed(name) && strings.TrimSpace(runtime.Str(name)) != ""
}

var SheetCreateFilterView = common.Shortcut{
	Service:     "sheets",
	Command:     "+create-filter-view",
	Description: "Create a filter view",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL (required if --spreadsheet-token is not set)"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token (required if --url is not set)"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "range", Desc: "filter range (e.g. sheetId!A1:H14)", Required: true},
		{Name: "filter-view-name", Desc: "display name (max 100 chars)"},
		{Name: "filter-view-id", Desc: "custom 10-char alphanumeric ID (auto-generated if omitted)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateFilterViewToken(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("range")) == "" {
			return common.FlagErrorf("--range must not be empty")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFilterViewToken(runtime)
		body := map[string]interface{}{"range": runtime.Str("range")}
		if s := runtime.Str("filter-view-name"); s != "" {
			body["filter_view_name"] = s
		}
		if s := runtime.Str("filter-view-id"); s != "" {
			body["filter_view_id"] = s
		}
		return common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views").
			Body(body).Set("token", token).Set("sheet_id", runtime.Str("sheet-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		body := map[string]interface{}{"range": runtime.Str("range")}
		if s := runtime.Str("filter-view-name"); s != "" {
			body["filter_view_name"] = s
		}
		if s := runtime.Str("filter-view-id"); s != "" {
			body["filter_view_id"] = s
		}
		data, err := runtime.CallAPI("POST", filterViewBasePath(token, runtime.Str("sheet-id")), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetUpdateFilterView = common.Shortcut{
	Service:     "sheets",
	Command:     "+update-filter-view",
	Description: "Update a filter view",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL (required if --spreadsheet-token is not set)"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token (required if --url is not set)"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
		{Name: "filter-view-id", Desc: "filter view ID", Required: true},
		{Name: "range", Desc: "new filter range"},
		{Name: "filter-view-name", Desc: "new display name (max 100 chars)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateFilterViewToken(runtime); err != nil {
			return err
		}
		if !hasNonEmptyStringFlag(runtime, "range") &&
			!hasNonEmptyStringFlag(runtime, "filter-view-name") {
			return common.FlagErrorf("specify at least one of --range or --filter-view-name")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFilterViewToken(runtime)
		body := map[string]interface{}{}
		if s := runtime.Str("range"); s != "" {
			body["range"] = s
		}
		if s := runtime.Str("filter-view-name"); s != "" {
			body["filter_view_name"] = s
		}
		return common.NewDryRunAPI().
			PATCH("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id").
			Body(body).Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).Set("filter_view_id", runtime.Str("filter-view-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		body := map[string]interface{}{}
		if s := runtime.Str("range"); s != "" {
			body["range"] = s
		}
		if s := runtime.Str("filter-view-name"); s != "" {
			body["filter_view_name"] = s
		}
		data, err := runtime.CallAPI("PATCH", filterViewItemPath(token, runtime.Str("sheet-id"), runtime.Str("filter-view-id")), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetListFilterViews = common.Shortcut{
	Service:     "sheets",
	Command:     "+list-filter-views",
	Description: "List all filter views in a sheet",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "spreadsheet URL (required if --spreadsheet-token is not set)"},
		{Name: "spreadsheet-token", Desc: "spreadsheet token (required if --url is not set)"},
		{Name: "sheet-id", Desc: "sheet ID", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := validateFilterViewToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := validateFilterViewToken(runtime)
		return common.NewDryRunAPI().
			GET("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/query").
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		data, err := runtime.CallAPI("GET", filterViewBasePath(token, runtime.Str("sheet-id"))+"/query", nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetGetFilterView = common.Shortcut{
	Service:     "sheets",
	Command:     "+get-filter-view",
	Description: "Get a filter view by ID",
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
			GET("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id").
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).Set("filter_view_id", runtime.Str("filter-view-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		data, err := runtime.CallAPI("GET", filterViewItemPath(token, runtime.Str("sheet-id"), runtime.Str("filter-view-id")), nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var SheetDeleteFilterView = common.Shortcut{
	Service:     "sheets",
	Command:     "+delete-filter-view",
	Description: "Delete a filter view",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
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
			DELETE("/open-apis/sheets/v3/spreadsheets/:token/sheets/:sheet_id/filter_views/:filter_view_id").
			Set("token", token).Set("sheet_id", runtime.Str("sheet-id")).Set("filter_view_id", runtime.Str("filter-view-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, _ := validateFilterViewToken(runtime)
		data, err := runtime.CallAPI("DELETE", filterViewItemPath(token, runtime.Str("sheet-id"), runtime.Str("filter-view-id")), nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
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
		if !hasNonEmptyStringFlag(runtime, "filter-type") &&
			!hasNonEmptyStringFlag(runtime, "compare-type") &&
			!hasNonEmptyStringFlag(runtime, "expected") {
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
		_ = json.Unmarshal([]byte(s), &arr)
		body["expected"] = arr
	}
	return body
}
