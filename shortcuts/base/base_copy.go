// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseCopy = common.Shortcut{
	Service:     "base",
	Command:     "+base-copy",
	Description: "Copy a base resource",
	Risk:        "write",
	UserScopes:  []string{"base:app:copy"},
	BotScopes:   []string{"base:app:copy", "docs:permission.member:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "name", Desc: "new base name"},
		{Name: "folder-token", Desc: "folder token for destination"},
		{Name: "without-content", Type: "bool", Desc: "copy structure only"},
		{Name: "time-zone", Desc: "time zone, e.g. Asia/Shanghai"},
	},
	DryRun: dryRunBaseCopy,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseCopy(runtime)
	},
}
