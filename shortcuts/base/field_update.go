// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+field-update",
	Description: "Update a field by ID or name",
	Risk:        "write",
	Scopes:      []string{"base:field:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
		{Name: "json", Desc: `field property JSON object, e.g. {"name":"Status","type":"text"}; read skills/lark-base/references/lark-base-field-update.md`, Required: true},
		{Name: "i-have-read-guide", Type: "bool", Desc: "acknowledge reading formula/lookup guide before creating or updating those field types", Hidden: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFieldUpdate(runtime)
	},
	DryRun: dryRunFieldUpdate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldUpdate(runtime)
	},
}
