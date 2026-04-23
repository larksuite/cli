// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"

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

func validateFilterViewToken(runtime *common.RuntimeContext) (string, error) {
	token := runtime.Str("spreadsheet-token")
	if runtime.Str("url") != "" {
		token = extractSpreadsheetToken(runtime.Str("url"))
	}
	if token == "" {
		return "", common.FlagErrorf("specify --url or --spreadsheet-token")
	}
	return token, nil
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
		_, err := validateFilterViewToken(runtime)
		return err
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
		if !runtime.Cmd.Flags().Changed("range") &&
			!runtime.Cmd.Flags().Changed("filter-view-name") {
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
