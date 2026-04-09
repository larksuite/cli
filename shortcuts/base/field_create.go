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
		{Name: "json", Desc: "field property JSON object", Required: true},
		{Name: "i-have-read-guide", Type: "bool", Desc: "set only after you have read the formula/lookup guide for those field types", Hidden: true},
	},
	Tips: []string{
		`Example: --json '{"name":"Status","type":"text"}'`,
		"Agent hint: use the lark-base skill's field-create guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFieldCreate(runtime)
	},
	DryRun: dryRunFieldCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldCreate(runtime)
	},
}
