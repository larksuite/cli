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
	Risk:        common.RiskRead,
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Aliases: []string{"page-size"}, Type: "int", Default: "100", Desc: "pagination size, range 1-200"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidatePageSizeTyped(runtime, "limit", 100, 1, 200)
		return err
	},
	DryRun: dryRunFieldList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldList(runtime)
	},
}
