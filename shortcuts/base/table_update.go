// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseTableUpdate = common.Shortcut{
	Service:           "base",
	Command:           "+table-update",
	Description:       "Rename a table by ID or name",
	Risk:              "write",
	ConditionalScopes: []string{"wiki:node:retrieve"},
	Scopes:            []string{"base:table:update"},
	AuthTypes:         authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "name", Desc: "new table name", Required: true},
	},
	DryRun: dryRunTableUpdate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableUpdate(runtime)
	},
}
