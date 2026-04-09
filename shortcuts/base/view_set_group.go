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
		{Name: "json", Desc: `group JSON object/array, e.g. {"group_config":[{"field":"fldStatus","desc":false}]}; read skills/lark-base/references/lark-base-view-set-group.md`, Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONValue(runtime)
	},
	DryRun: dryRunViewSetGroup,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetWrapped(runtime, "group", "group_config", "group")
	},
}
