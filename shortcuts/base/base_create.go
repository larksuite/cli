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
	UserScopes:  []string{"base:app:create"},
	BotScopes:   []string{"base:app:create", "docs:permission.member:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		{Name: "name", Desc: "base name", Required: true},
		{Name: "folder-token", Desc: "folder token for destination"},
		{Name: "time-zone", Desc: "time zone, e.g. Asia/Shanghai"},
	},
	DryRun: dryRunBaseCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseCreate(runtime)
	},
}
