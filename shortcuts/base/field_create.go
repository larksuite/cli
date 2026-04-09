// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldCreate = common.Shortcut{
	Service:     "base",
	Command:     "+field-create",
	Description: "Create a field",
	Risk:        "write",
	Scopes:      []string{"base:field:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: `field property JSON object, e.g. {"name":"Status","type":"text"}; read skills/lark-base/references/lark-base-field-create.md`, Required: true},
		{Name: "i-have-read-guide", Type: "bool", Desc: "set only after you have read the formula/lookup guide for those field types", Hidden: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFieldCreate(runtime)
	},
	DryRun: dryRunFieldCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldCreate(runtime)
	},
}
