// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewSetCard = common.Shortcut{
	Service:     "base",
	Command:     "+view-set-card",
	Description: "Set view card configuration",
	Risk:        "write",
	Scopes:      []string{"base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		viewRefFlag(true),
		{Name: "json", Desc: "card JSON object", Required: true},
	},
	Tips: []string{
		`Example: --json '{"cover_field":"fldCover"}'`,
		"Agent hint: use the lark-base skill's view-set-card guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONObject(runtime)
	},
	DryRun: dryRunViewSetCard,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetJSONObject(runtime, "card", "card")
	},
}
