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
		{Name: "json", Desc: `timebar JSON object with start_time, end_time, title, e.g. {"start_time":"Start Date","end_time":"End Date","title":"Name"}`, Required: true},
	},
	Tips: []string{
		"Supported view types: calendar, gantt.",
		"start_time, end_time, and title are required; use date/time fields for start_time and end_time.",
		"Use +view-get-timebar first when modifying an existing timebar configuration.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONObject(runtime)
	},
	DryRun: dryRunViewSetTimebar,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetJSONObject(runtime, "timebar", "timebar")
	},
}
