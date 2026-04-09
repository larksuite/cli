// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewSetSort = common.Shortcut{
	Service:     "base",
	Command:     "+view-set-sort",
	Description: "Set view sort configuration",
	Risk:        "write",
	Scopes:      []string{"base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		viewRefFlag(true),
		{Name: "json", Desc: `sort JSON object/array, e.g. [{"field":"fldPriority","desc":true}]; read skills/lark-base/references/lark-base-view-set-sort.md`, Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONValue(runtime)
	},
	DryRun: dryRunViewSetSort,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetWrapped(runtime, "sort", "sort_config", "sort")
	},
}
