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
	Description: "List all fields in a table",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "offset", Type: "int", Default: "0", Desc: "legacy pagination offset; hidden for compatibility", Hidden: true},
		{Name: "limit", Aliases: []string{"page-size"}, Type: "int", Default: "300", Desc: "legacy pagination size, range 1-300; hidden for compatibility", Hidden: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidatePageSizeTyped(runtime, "limit", 300, 1, 300)
		return err
	},
	DryRun: dryRunFieldList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldList(runtime)
	},
}
