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
		{Name: "json", Desc: `visible fields JSON object with "visible_fields"`, Required: true},
	},
	Tips: []string{
		`Example: --json '{"visible_fields":["fldXXX"]}'`,
		"Agent hint: use the lark-base skill's view-set-visible-fields guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONObject(runtime)
	},
	DryRun: dryRunViewSetVisibleFields,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetVisibleFields(runtime)
	},
}
