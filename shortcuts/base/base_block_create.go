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
	Description: "Create a base block",
	Risk:        "write",
	Scopes:      []string{"base:app:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "type", Desc: "resource type", Required: true, Enum: baseBlockTypeEnums},
		{Name: "name", Desc: "base block name", Required: true},
		{Name: "parent-id", Desc: "folder base block id; when omitted, create at the base root"},
	},
	Tips: []string{
		"Use this for Base container entries. Use table/field/record/docx/dashboard/workflow commands for content inside the created entity.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateBaseBlockCreate(runtime)
	},
	DryRun: dryRunBaseBlockCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseBlockCreate(runtime)
	},
}
