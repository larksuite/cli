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
		{Name: "json", Desc: `sort_config JSON object, e.g. {"sort_config":[{"field":"Priority","desc":true}]}; use {"sort_config":[]} to clear; max 10 items`, Required: true},
	},
	Tips: []string{
		"Supported view types: grid, kanban, gallery, gantt.",
		"Use a JSON object, not a bare array; sorting fields must be supported by the current view.",
		"sort_config supports max 10 sort items.",
		"Use +view-get-sort first when modifying an existing sort configuration.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONObject(runtime)
	},
	DryRun: dryRunViewSetSort,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetWrapped(runtime, "sort", "sort_config", "sort")
	},
}
