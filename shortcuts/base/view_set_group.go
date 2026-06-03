// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewSetGroup = common.Shortcut{
	Service:     "base",
	Command:     "+view-set-group",
	Description: "Set view group configuration",
	Risk:        "write",
	Scopes:      []string{"base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		viewRefFlag(true),
		{Name: "json", Desc: `group JSON object with group_config array, e.g. {"group_config":[{"field":"Status","desc":false}]}; use {"group_config":[]} to clear`, Required: true},
	},
	Tips: []string{
		"Supported view types: grid, kanban, gantt.",
		"Use a JSON object, not a bare array; grouping fields must be supported by the current view.",
		"group_config supports max 3 group items.",
		"Use +view-get-group first when modifying an existing grouping configuration.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONObject(runtime)
	},
	DryRun: dryRunViewSetGroup,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetWrapped(runtime, "group", "group_config", "group")
	},
}
