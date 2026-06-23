// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewRename = common.Shortcut{
	Service:           "base",
	Command:           "+view-rename",
	Description:       "Rename a view by ID or name",
	Risk:              "write",
	ConditionalScopes: []string{"wiki:node:retrieve"},
	Scopes:            []string{"base:view:write_only"},
	AuthTypes:         authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		viewRefFlag(true),
		{Name: "name", Desc: "new view name", Required: true},
	},
	DryRun: dryRunViewRename,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewRename(runtime)
	},
}
