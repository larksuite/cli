// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseTableList = common.Shortcut{
	Service:     "base",
	Command:     "+table-list",
	Description: "List tables in a base",
	Risk:        "read",
	Scopes:      []string{"base:table:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "offset", Aliases: []string{"next-token", "page-token"}, Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Aliases: []string{"page-size"}, Type: "int", Default: "50", Desc: "pagination size, range 1-100"},
	},
	Tips: []string{
		"When meta.pagination.complete is false, continue with `lark-cli base +table-list --base-token <base> --offset <next_token>` until complete is true. The decimal next_token response value maps to canonical --offset; response keys are not flags to guess. Compatibility aliases --next-token and --page-token are also accepted.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidatePageSizeTyped(runtime, "limit", 50, 1, 100)
		return err
	},
	DryRun: dryRunTableList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableList(runtime)
	},
}
