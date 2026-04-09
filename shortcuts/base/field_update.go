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
		{Name: "json", Desc: "field property JSON object", Required: true},
		{Name: "i-have-read-guide", Type: "bool", Desc: "acknowledge reading formula/lookup guide before creating or updating those field types", Hidden: true},
	},
	Tips: []string{
		`Example: --json '{"name":"Status","type":"text"}'`,
		"Agent hint: use the lark-base skill's field-update guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFieldUpdate(runtime)
	},
	DryRun: dryRunFieldUpdate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldUpdate(runtime)
	},
}
