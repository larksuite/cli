// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseTableGet = common.Shortcut{
	Service:     "base",
	Command:     "+table-get",
	Description: "Get a table by ID or name",
	Risk:        "read",
	Scopes:      []string{"base:table:read", "base:field:read", "base:view:read"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true), tableRefFlag(true)},
	Tips: []string{
		`Example: lark-cli base +table-get --base-token <base_token> --table-id "Tasks"`,
		"table-id accepts a table ID (tbl...) or the table name in the current Base.",
	},
	DryRun: dryRunTableGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableGet(runtime)
	},
}
