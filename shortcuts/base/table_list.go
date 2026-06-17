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
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "50", Desc: "pagination size, range 1-100"},
		pageSizeLimitAliasFlag(),
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateLimitPageSizeAlias(runtime); err != nil {
			return err
		}
		if _, err := common.ValidatePageSizeTyped(runtime, "limit", 50, 1, 100); err != nil {
			return err
		}
		if runtime.Changed("page-size") {
			if _, err := common.ValidatePageSizeTyped(runtime, "page-size", 50, 1, 100); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: dryRunTableList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableList(runtime)
	},
}
