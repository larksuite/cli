// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldListBatch = common.Shortcut{
	Service:     "base",
	Command:     "+field-list-batch",
	Description: "List fields for multiple tables in one call",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "table-id", Type: "string_array", Desc: tableRefFlag(true).Desc + "; repeat to list fields for multiple tables", Required: true},
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "100", Desc: "pagination size, range 1-200"},
		{Name: "compact", Type: "bool", Desc: "return compact field objects (id/name/type/style/options) for lower context cost; default returns full field objects"},
	},
	DryRun: dryRunFieldListBatch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldListBatch(runtime)
	},
}
