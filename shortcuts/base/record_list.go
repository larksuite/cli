// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

var BaseRecordList = common.Shortcut{
	Service:     "base",
	Command:     "+record-list",
	Description: "List records in a table",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		recordListFieldRefFlag(),
		{Name: "field-names", Type: "string_array", Hidden: true},
		{Name: "fields", Type: "string_array", Hidden: true},
		recordListViewRefFlag(),
		recordFilterFlag(),
		recordFilterAliasFlag(),
		recordSortFlag(),
		recordSortAliasFlag(),
		{Name: "json", Hidden: true},
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "100", Desc: "pagination size, range 1-200"},
		recordReadFormatFlag(),
	},
	Tips: []string{
		"Example: lark-cli base +record-list --base-token <base_token> --table-id <table_id> --limit 50",
		"Example with projection: lark-cli base +record-list --base-token <base_token> --table-id <table_id> --field-id Name --field-id Status --limit 50",
		`Text equality filter: --filter-json '{"logic":"and","conditions":[["Title","==","Launch plan"]]}'`,
		`Text contains/like filter: --filter-json '{"logic":"and","conditions":[["Title","intersects","urgent"]]}'`,
		`Number equality filter: --filter-json '{"logic":"and","conditions":[["Score","==",95]]}'`,
		`Date equality filter: --filter-json '{"logic":"and","conditions":[["Due Date","==","ExactDate(2026-06-02)"]]}'`,
		`Option intersection filter: --filter-json '{"logic":"and","conditions":[["Tags","intersects",["P0","Blocked"]]]}'`,
		`Sort priority follows --sort-json array order: --sort-json '[{"field":"Updated","desc":true},{"field":"Title","desc":false}]'`,
		formatRecordQueryPriorityTip(),
		"Default output is markdown; pass --format json to get the raw JSON envelope.",
		"Use --field-id repeatedly to keep output small and aligned with the task.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateRecordReadFormat(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("json")) != "" {
			return baseFlagErrorf("+record-list does not support --json; use --filter-json for filters and --sort-json for sorting")
		}
		return validateRecordQueryOptions(runtime)
	},
	DryRun: dryRunRecordList,
	PostMount: func(cmd *cobra.Command) {
		preserveFlagOrder(cmd)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordList(runtime)
	},
}

func recordListFieldRefFlag() common.Flag {
	flag := fieldRefFlag(false)
	flag.Type = "string_array"
	flag.Desc = "field ID or name to include; repeat to project only needed fields"
	return flag
}

func recordListViewRefFlag() common.Flag {
	flag := viewRefFlag(false)
	flag.Desc = "view ID or name; omit for reading all table records, or set to read a user-specified or temporary filtered/sorted view"
	return flag
}

func recordReadFormatFlag() common.Flag {
	return common.Flag{
		Name:    "format",
		Default: "markdown",
		Desc:    "output format: markdown (default) | json",
	}
}
