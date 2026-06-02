// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseBlockMove = common.Shortcut{
	Service:     "base",
	Command:     "+base-block-move",
	Description: "Move a block",
	Risk:        "write",
	Scopes:      []string{"base:block:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		baseBlockIDFlag(true),
		{Name: "parent-id", Desc: "target folder block id; when omitted, move to root"},
		{Name: "before-id", Desc: "sibling block id; move the block before this sibling in the target folder/root order"},
		{Name: "after-id", Desc: "sibling block id; move the block after this sibling in the target folder/root order"},
	},
	Tips: []string{
		"Example: lark-cli base +base-block-move --base-token <base_token> --block-id <block_id> --parent-id <folder_block_id>",
		"Example: lark-cli base +base-block-move --base-token <base_token> --block-id <block_id> --after-id <sibling_block_id>",
		"Example: lark-cli base +base-block-move --base-token <base_token> --block-id <block_id> --before-id <sibling_block_id>",
		"Example: lark-cli base +base-block-move --base-token <base_token> --block-id <block_id>",
		"Omit --parent-id to move the block to root; do not pass null.",
		"--before-id and --after-id are mutually exclusive.",
		"When moving a folder, its children remain under that folder.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateBaseBlockMove(runtime)
	},
	DryRun: dryRunBaseBlockMove,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockMove(runtime)
	},
}
