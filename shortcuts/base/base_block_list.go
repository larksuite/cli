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
	Description: "List blocks in a base",
	Risk:        "read",
	Scopes:      []string{"base:block:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "type", Desc: "filter by resource type", Enum: baseBlockTypeEnums},
		{Name: "parent-id", Desc: "folder block id; when omitted, list all blocks"},
	},
	Tips: []string{
		"Blocks are entries in the base sidebar, such as folder, table, docx, dashboard, and workflow.",
		"For table, dashboard, and workflow blocks, returned id is the table-id, dashboard-id, or workflow-id used by the corresponding commands.",
		"For docx blocks, use the returned docx_token with docx commands.",
		"This command returns the full backend list. It intentionally does not expose limit or offset.",
		"Pass --type to list only one resource type.",
		"Pass --parent-id to list only direct children of a folder.",
		"Dashboard blocks are chart/widget blocks inside a dashboard; use +dashboard-block-* for those.",
	},
	DryRun: dryRunBaseBlockList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockList(runtime)
	},
}
