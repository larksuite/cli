// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseCreate = common.Shortcut{
	Service:     "base",
	Command:     "+base-create",
	Description: "Create a new base resource",
	Risk:        "write",
	Scopes:      []string{"base:app:create", "bitable:app.table:read", "bitable:app.table.record:delete"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		{Name: "name", Desc: "base name", Required: true},
		{Name: "folder-token", Desc: "folder token for destination"},
		{Name: "time-zone", Desc: "time zone, e.g. Asia/Shanghai"},
		{Name: "keep-empty-rows", Type: "bool", Default: "false", Desc: "retain the default 5 empty rows in the newly created base"},
	},
	DryRun: dryRunBaseCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseCreate(runtime)
	},
}
