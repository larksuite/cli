// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

var BaseRecordGet = common.Shortcut{
	Service:     "base",
	Command:     "+record-get",
	Description: "Get one or more records by ID",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "record-id", Type: "string_array", Desc: "record ID (repeatable)"},
		{Name: "field-id", Type: "string_array", Desc: "field ID or name to project; repeat to keep only needed columns"},
		{Name: "json", Desc: `JSON object with record_id_list, e.g. {"record_id_list":["rec_xxx"]}`},
		recordReadFormatFlag(),
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateRecordReadFormat(runtime); err != nil {
			return err
		}
		return validateRecordSelection(runtime)
	},
	Tips: []string{
		"Example: lark-cli base +record-get --base-token <base_token> --table-id <table_id> --record-id <record_id>",
		"Example with projection: lark-cli base +record-get --base-token <base_token> --table-id <table_id> --record-id rec_001 --record-id rec_002 --field-id Name --field-id Status",
		"Default output is markdown; pass --format json to get the raw JSON envelope.",
		"Use --field-id as a projection boundary to avoid loading large cell values into context when they are not needed.",
		"Use +record-get when record_id is already known; otherwise use +record-search or +record-list.",
		"Agent hint: follow the lark-base record read SOP for record read routing.",
	},
	DryRun: dryRunRecordGet,
	PostMount: func(cmd *cobra.Command) {
		preserveFlagOrder(cmd)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordGet(runtime)
	},
}
