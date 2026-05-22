// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseBlockRename = common.Shortcut{
	Service:     "base",
	Command:     "+base-block-rename",
	Description: "Rename a base block",
	Risk:        "write",
	Scopes:      []string{"base:block:write"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		baseBlockIDFlag(true),
		{Name: "name", Desc: "new base block name", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateBaseBlockRename(runtime)
	},
	DryRun: dryRunBaseBlockRename,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockRename(runtime)
	},
}
