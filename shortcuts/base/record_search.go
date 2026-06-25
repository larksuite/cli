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
		{Name: "json", Desc: `record search JSON object for the full request body, e.g. {"keyword":"Alice","search_fields":["Name"],"select_fields":["Name","Status"],"filter":{"logic":"and","conditions":[]},"sort":[{"field":"Updated","desc":true}],"limit":50}; escape hatch for advanced cases`},
		{Name: "keyword", Desc: "keyword for record search; required unless --json is used"},
		{Name: "search-field", Type: "string_array", Desc: "field ID or name to search; repeat for multiple fields; required unless --json is used"},
		recordListFieldRefFlag(),
		recordListViewRefFlag(),
		recordFilterFlag(),
		recordSortFlag(),
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "10", Desc: "pagination size, range 1-200"},
		pageSizeLimitAliasFlag(),
		recordReadFormatFlag(),
	},
	Tips: []string{
		`Happy path fields: keyword (string), search_fields (1-20 field names/ids), select_fields (optional projection, <=50), view_id (optional), offset (default 0), limit (default 10, range 1-200).`,
		"JSON constraints: keyword length >=1; search_fields length 1-20; select_fields length <=50; offset >=0 defaults to 0; limit range 1-200 defaults to 10.",
		"view_id scopes search to records in that view; when select_fields is omitted, returned fields follow that view's visible fields.",
		`Example: lark-cli base +record-search --base-token <base_token> --table-id <table_id> --keyword Alice --search-field Name --field-id Name --field-id Status --limit 20`,
		`Example with filter/sort JSON: lark-cli base +record-search --base-token <base_token> --table-id <table_id> --keyword Alice --search-field Name --filter-json @filter.json --sort-json '[{"field":"Updated","desc":true}]'`,
		`Text equality filter: --filter-json '{"logic":"and","conditions":[["Title","==","Launch plan"]]}'`,
		`Text contains/like filter: --filter-json '{"logic":"and","conditions":[["Title","intersects","urgent"]]}'`,
		`Option intersection filter: --filter-json '{"logic":"and","conditions":[["Tags","intersects",["P0","Blocked"]]]}'`,
		`Sort priority follows --sort-json array order.`,
		formatRecordQueryPriorityTip(),
		"Use +record-search for keyword matching; use --filter-json for structured conditions and --sort-json for result ordering.",
		"Use --json only when you need to pass the full search body directly.",
		"Default output is markdown; pass --format json to get the raw JSON envelope.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordSearchFlags(runtime)
	},
	DryRun: dryRunRecordSearch,
	PostMount: func(cmd *cobra.Command) {
		preserveFlagOrder(cmd)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordSearch(runtime)
	},
}
