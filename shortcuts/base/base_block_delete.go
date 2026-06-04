// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseBlockDelete = common.Shortcut{
	Service:     "base",
	Command:     "+base-block-delete",
	Description: "Delete a block",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:block:delete"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		baseBlockIDFlag(true),
	},
	Tips: []string{
		"Example: lark-cli base +base-block-delete --base-token <base_token> --block-id <block_id> --yes",
		"Deletes the block identified by --block-id.",
		"Recursive folder deletion is not supported. If a folder is not empty, move or delete its children first.",
		"Different block types may have independent backing resources; deletion follows backend semantics.",
		"Use +base-block-list first when you need to confirm the target block id.",
		"If the user already explicitly confirmed this exact delete target, pass --yes without asking again.",
	},
	DryRun: dryRunBaseBlockDelete,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockDelete(runtime)
	},
}
