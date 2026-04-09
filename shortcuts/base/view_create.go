// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseViewCreate = common.Shortcut{
	Service:     "base",
	Command:     "+view-create",
	Description: "Create one or more views",
	Risk:        "write",
	Scopes:      []string{"base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: "view JSON object/array", Required: true},
	},
	Tips: []string{
		`Example: --json '{"name":"Main","type":"grid"}'`,
		"Agent hint: use the lark-base skill's view-create guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateViewCreate(runtime)
	},
	DryRun: dryRunViewCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeViewCreate(runtime)
	},
}
