// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordGet = common.Shortcut{
	Service:     "base",
	Command:     "+record-get",
	Description: "Get a record by ID",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		recordRefFlag(true),
		{Name: "field-id", Type: "string_array", Desc: "field ID or field name to include (repeatable)"},
	},
	DryRun: dryRunRecordGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordGet(runtime)
	},
}
