// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewSetTimebar = common.Shortcut{
	Service:     "base",
	Command:     "+view-set-timebar",
	Description: "Set view timebar configuration",
	Risk:        "write",
	Scopes:      []string{"base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		viewRefFlag(true),
		{Name: "json", Desc: "timebar JSON object", Required: true},
	},
	Tips: []string{
		`Example: --json '{"start_time":"fldStart","end_time":"fldEnd","title":"fldTitle"}'`,
		"Agent hint: use the lark-base skill's view-set-timebar guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONObject(runtime)
	},
	DryRun: dryRunViewSetTimebar,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetJSONObject(runtime, "timebar", "timebar")
	},
}
