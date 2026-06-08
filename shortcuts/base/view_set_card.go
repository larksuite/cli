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
		{Name: "json", Desc: `card JSON object, e.g. {"cover_field":"Cover"} or {"cover_field":null} to clear`, Required: true},
	},
	Tips: []string{
		"Supported view types: gallery, kanban.",
		"cover_field should be an attachment field id/name, or null to clear.",
		"Use +view-get-card first when updating an existing card view configuration.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewJSONObject(runtime)
	},
	DryRun: dryRunViewSetCard,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewSetJSONObject(runtime, "card", "card")
	},
}
