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
	Description: "Move a base block",
	Risk:        "write",
	Scopes:      []string{"base:block:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		baseBlockIDFlag(true),
		{Name: "parent-id", Desc: "target folder base block id; when omitted, move to the base root"},
		{Name: "before-id", Desc: "place before this sibling base block id"},
		{Name: "after-id", Desc: "place after this sibling base block id"},
	},
	Tips: []string{
		"Omit --parent-id to move the block to the base root; do not pass null.",
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
