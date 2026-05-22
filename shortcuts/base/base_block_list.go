// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseBlockList = common.Shortcut{
	Service:     "base",
	Command:     "+base-block-list",
	Description: "List base blocks in a base",
	Risk:        "read",
	Scopes:      []string{"base:block:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "parent-id", Desc: "folder base block id; when omitted, list all base blocks"},
	},
	Tips: []string{
		"Base blocks are entries managed by the Base container, such as folder, table, docx, dashboard, and workflow.",
		"This command returns the full backend list. It intentionally does not expose limit or offset.",
		"Pass --parent-id to list only direct children of a folder base block.",
		"Dashboard blocks are chart/widget blocks inside a dashboard; use +dashboard-block-* for those.",
	},
	DryRun: dryRunBaseBlockList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockList(runtime)
	},
}
