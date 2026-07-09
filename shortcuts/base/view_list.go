// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewList = common.Shortcut{
	Service:     "base",
	Command:     "+view-list",
	Description: "List views in a table",
	Risk:        "read",
	Scopes:      []string{"base:view:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "100", Desc: "pagination size, range 1-200"},
		pageSizeLimitAliasFlag(),
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateLimitPageSizeAlias(runtime); err != nil {
			return err
		}
		if _, err := common.ValidatePageSizeTyped(runtime, "limit", 100, 1, 200); err != nil {
			return err
		}
		if runtime.Changed("page-size") {
			if _, err := common.ValidatePageSizeTyped(runtime, "page-size", 100, 1, 200); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: dryRunViewList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewList(runtime)
	},
}
