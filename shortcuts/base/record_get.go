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
	Description: "Get a record by ID",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		recordRefFlag(true),
	},
	Tips: []string{
		"Example: lark-cli base +record-get --base-token <base_token> --table-id <table_id> --record-id <record_id>",
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
