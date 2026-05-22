// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseBlockCreate = common.Shortcut{
	Service:     "base",
	Command:     "+base-block-create",
	Description: "Create a block",
	Risk:        "write",
	Scopes:      []string{"base:block:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "type", Desc: "resource type", Required: true, Enum: baseBlockTypeEnums},
		{Name: "name", Desc: "block name", Required: true},
		{Name: "parent-id", Desc: "folder block id; when omitted, create at root"},
	},
	Tips: []string{
		"Creates a folder, table, docx, dashboard, or workflow entry.",
		"Do not pass null for --parent-id. Omit it to create at the root level.",
		"Created resources still use their own commands for content operations, such as table/field/record/docx/dashboard/workflow commands.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateBaseBlockCreate(runtime)
	},
	DryRun: dryRunBaseBlockCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockCreate(runtime)
	},
}
