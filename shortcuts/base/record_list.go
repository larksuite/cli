// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

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
		recordListViewRefFlag(),
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "100", Desc: "pagination size, range 1-200"},
		recordReadFormatFlag(),
	},
	Tips: []string{
		"Example: lark-cli base +record-list --base-token <base_token> --table-id <table_id> --limit 50",
		"Example with projection: lark-cli base +record-list --base-token <base_token> --table-id <table_id> --field-id Name --field-id Status --limit 50",
		"Default output is markdown; pass --format json to get the raw JSON envelope.",
		"Use --field-id repeatedly to keep output small and aligned with the task.",
		"Use --view-id when the user asks for a specific view or after creating a temporary filtered/sorted view.",
		"For structured filters, sorting, Top/Bottom N, and link fields, follow the lark-base record read SOP.",
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
