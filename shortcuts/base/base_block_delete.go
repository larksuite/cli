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
	Description: "Delete a base block",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:app:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		baseBlockIDFlag(true),
	},
	Tips: []string{
		"Recursive folder deletion is not supported. If a folder is not empty, move or delete its children first.",
	},
	DryRun: dryRunBaseBlockDelete,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockDelete(runtime)
	},
}
