// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

var BaseRecordSearch = common.Shortcut{
	Service:     "base",
	Command:     "+record-search",
	Description: "Search records in a table",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: `record search JSON object, e.g. {"keyword":"Alice","search_fields":["Name"],"select_fields":["Name","Status"],"limit":50}; for keyword search only`, Required: true},
		recordReadFormatFlag(),
	},
	Tips: []string{
		`Happy path fields: keyword (string), search_fields (1-20 field names/ids), select_fields (optional projection, <=50), view_id (optional), offset (default 0), limit (default 10, range 1-200).`,
		"JSON constraints: keyword length >=1; search_fields length 1-20; select_fields length <=50; offset >=0 defaults to 0; limit range 1-200 defaults to 10.",
		"view_id scopes search to records in that view; when select_fields is omitted, returned fields follow that view's visible fields.",
		"Default output is markdown; pass --format json to get the raw JSON envelope.",
		"Use +record-search only for keyword search; for structured conditions, sorting, Top/Bottom N, or global conclusions, follow the lark-base record read SOP instead of inventing search JSON.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateRecordReadFormat(runtime); err != nil {
			return err
		}
		return validateRecordJSON(runtime)
	},
	DryRun: dryRunRecordSearch,
	PostMount: func(cmd *cobra.Command) {
		preserveFlagOrder(cmd)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordSearch(runtime)
	},
}
