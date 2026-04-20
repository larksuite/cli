// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewSetFilter = common.Shortcut{
	Service:     "base",
	Command:     "+view-set-filter",
	Description: "Set view filter configuration",
	Risk:        "write",
	Scopes:      []string{"base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		viewRefFlag(true),
		{Name: "json", Desc: "filter JSON object", Required: true},
	},
	Tips: []string{
		`Example: --json '{"logic":"and","conditions":[["fldStatus","==","Todo"]]}'`,
		"Agent hint: use the lark-base skill's view-set-filter guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONObject(runtime)
	},
	DryRun: dryRunViewSetFilter,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetJSONObject(runtime, "filter", "filter")
	},
}
