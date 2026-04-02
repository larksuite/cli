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
		{Name: "json", Desc: "visible fields JSON object/array", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONValue(runtime)
	},
	DryRun: dryRunViewSetVisibleFields,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetWrapped(runtime, "visible_fields", "visible_fields", "visible_fields")
	},
}
