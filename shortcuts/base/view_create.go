// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewCreate = common.Shortcut{
	Service:     "base",
	Command:     "+view-create",
	Description: "Create one or more views",
	Risk:        "write",
	Scopes:      []string{"base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: "view JSON object/array; type defaults to grid; type range: grid, kanban, gallery, calendar, gantt", Required: true},
	},
	Tips: []string{
		`Example: lark-cli base +view-create --base-token <base_token> --table-id <table_id> --json '{"name":"Main","type":"grid"}'`,
		`Minimal: --json '{"name":"Main"}' creates a grid view.`,
		"Do not pass form as a view type; form views are managed through form commands.",
		`Use +view-set-visible-fields after creation when the user needs a specific field order or visibility.`,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewCreate(runtime)
	},
	DryRun: dryRunViewCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewCreate(runtime)
	},
}
