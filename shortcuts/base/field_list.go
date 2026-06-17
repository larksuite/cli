// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldList = common.Shortcut{
	Service:     "base",
	Command:     "+field-list",
	Description: "List fields in a table",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
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
	DryRun: dryRunFieldList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldList(runtime)
	},
}
