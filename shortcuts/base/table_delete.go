// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseTableDelete = common.Shortcut{
	Service:     "base",
	Command:     "+table-delete",
	Description: "Delete a table by ID or name",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:table:delete"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true), tableRefFlag(true)},
	Tips: []string{
		`Example: lark-cli base +table-delete --base-token <base_token> --table-id "Old Tasks" --yes`,
		"table-id accepts a table ID (tbl...) or the table name in the current Base.",
		baseHighRiskYesTip,
	},
	DryRun: dryRunTableDelete,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableDelete(runtime)
	},
}
