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
		{Name: "json", Desc: "sort_config JSON object", Required: true},
	},
	Tips: []string{
		`Example: --json '{"sort_config":[{"field":"fldPriority","desc":true}]}'`,
		"Agent hint: use the lark-base skill's view-set-sort guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONValue(runtime)
	},
	DryRun: dryRunViewSetSort,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetWrapped(runtime, "sort", "sort_config", "sort")
	},
}
