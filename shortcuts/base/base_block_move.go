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
	Scopes:      []string{"base:block:write"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		baseBlockIDFlag(true),
		{Name: "parent-id", Desc: "target folder base block id; when omitted, move to the base root"},
		{Name: "before-id", Desc: "place before this sibling base block id"},
		{Name: "after-id", Desc: "place after this sibling base block id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateBaseBlockMove(runtime)
	},
	DryRun: dryRunBaseBlockMove,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockMove(runtime)
	},
}
