// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewSetVisibleFields = common.Shortcut{
	Service:     "base",
	Command:     "+view-set-visible-fields",
	Description: "Set view visible fields",
	Risk:        "write",
	Scopes:      []string{"base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		viewRefFlag(true),
		{Name: "json", Desc: `visible fields JSON object, e.g. {"visible_fields":["Name","Status"]}`, Required: true},
	},
	Tips: []string{
		"Supported view types: grid, kanban, gallery, calendar, gantt.",
		"Use a JSON object, not a bare array; primary field may be forced to the first position by the API.",
		"visible_fields controls both visibility and order; include every field that should remain visible.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONObject(runtime)
	},
	DryRun: dryRunViewSetVisibleFields,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetVisibleFields(runtime)
	},
}
