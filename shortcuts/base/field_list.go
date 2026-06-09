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
		{Name: "table-id", Type: "string_array", Desc: "table ID (must start with tbl if ID) or name; repeat to list fields for multiple tables", Required: true},
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "100", Desc: "pagination size, range 1-200"},
		{Name: "full", Type: "bool", Desc: "return full field objects (style/property/formula/lookup internals); default returns compact id/name/type/options for lower context cost"},
	},
	DryRun: dryRunFieldList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldList(runtime)
	},
}
