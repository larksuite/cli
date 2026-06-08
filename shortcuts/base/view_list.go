// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewList = common.Shortcut{
	Service:     "base",
	Command:     "+view-list",
	Description: "List views in a table",
	Risk:        "read",
	Scopes:      []string{"base:view:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "100", Desc: "pagination size, range 1-200"},
	},
	DryRun: dryRunViewList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewList(runtime)
	},
}
