// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseTableList = common.Shortcut{
	Service:     "base",
	Command:     "+table-list",
	Description: "List all tables in a base",
	Risk:        "read",
	Scopes:      []string{"base:table:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "offset", Type: "int", Default: "0", Desc: "legacy pagination offset; hidden for compatibility", Hidden: true},
		{Name: "limit", Aliases: []string{"page-size"}, Type: "int", Default: "300", Desc: "legacy pagination size, range 1-300; hidden for compatibility", Hidden: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidatePageSizeTyped(runtime, "limit", 300, 1, 300)
		return err
	},
	DryRun: dryRunTableList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableList(runtime)
	},
}
