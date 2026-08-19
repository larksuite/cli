// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// BaseTableCreate creates a table with an explicit schema. --fields is required
// at the cli surface (cobra MarkFlagRequired); a missing flag fails before
// Validate runs with cobra's standard "required flag(s)" error (which the
// dispatcher classifies as a typed *errs.ValidationError). validateTableCreate
// still rejects blank, non-array and empty-array values, because cobra accepts
// --fields "" and --fields "[]" — both of which would reach the API without a
// fields body and get the platform default schema instead of the caller's.
var BaseTableCreate = common.Shortcut{
	Service:     "base",
	Command:     "+table-create",
	Description: "Create a table with an explicit field schema, plus optional views",
	Risk:        "write",
	Scopes:      []string{"base:table:create", "base:field:read", "base:field:create", "base:field:update", "base:view:write_only"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "name", Desc: "table name", Required: true},
		{Name: "view", Desc: "view JSON object/array for create"},
		{Name: "fields", Required: true, Desc: `field JSON array defining the table schema; must hold at least one field, e.g. [{"name":"Title","type":"text"},{"name":"Status","type":"select","multiple":false,"options":[{"name":"Todo"},{"name":"Done"}]}]`},
	},
	Tips: []string{
		"Before using --fields, read lark-base-field-schema.md or rely on the same field JSON shape used by +field-create; do not invent field properties.",
		"The first --fields item becomes the primary field.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateTableCreate(runtime)
	},
	DryRun: dryRunTableCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableCreate(runtime)
	},
}
