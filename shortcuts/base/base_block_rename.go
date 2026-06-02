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
	Description: "Rename a block",
	Risk:        "write",
	Scopes:      []string{"base:block:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		baseBlockIDFlag(true),
		{Name: "name", Desc: "new unique block name; must not duplicate another block name in this base", Required: true},
	},
	Tips: []string{
		"Example: lark-cli base +base-block-rename --base-token <base_token> --block-id <block_id> --name \"New name\"",
		"Renames the block identified by --block-id.",
		"Block names must be unique in the base; use +base-block-list first when you need to check existing names.",
		"Use +base-block-list first when you need to resolve the target block id from a visible name.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateBaseBlockRename(runtime)
	},
	DryRun: dryRunBaseBlockRename,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockRename(runtime)
	},
}
