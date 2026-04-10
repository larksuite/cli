// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordList = common.Shortcut{
	Service:     "base",
	Command:     "+record-list",
	Description: "List records in a table",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "field-id", Type: "string_array", Desc: "field ID or field name to include (repeatable)"},
		{Name: "view-id", Desc: "view ID"},
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "100", Desc: "pagination size"},
	},
	DryRun: dryRunRecordList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordList(runtime)
	},
}
