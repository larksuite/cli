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
	UserScopes: []string{
		"base:app:create",
		"base:table:read",
		"base:table:create",
		"base:table:update",
		"base:table:delete",
	},
	BotScopes: []string{
		"base:app:create",
		"base:table:read",
		"base:table:create",
		"base:table:update",
		"base:table:delete",
		"docs:permission.member:create",
	},
	AuthTypes: authTypes(),
	Flags: []common.Flag{
		{Name: "name", Desc: "base name", Required: true},
		{Name: "folder-token", Desc: "folder token for destination"},
		{Name: "time-zone", Desc: "time zone, e.g. Asia/Shanghai"},
		{Name: "fields", Desc: `field JSON array for the first table schema; use with --table-name, e.g. [{"name":"Title","type":"text"},{"name":"Status","type":"select","options":[{"name":"Todo"},{"name":"Done"}]}]`},
		{Name: "table-name", Desc: "first table name for the custom first table schema; use with --fields"},
	},
	Tips: []string{
		`Example: lark-cli base +base-create --name "Project Tracker" --time-zone Asia/Shanghai`,
		`Strongly recommended initial table schema: lark-cli base +base-create --name "Project Tracker" --table-name "Tasks" --fields '[{"name":"Title","type":"text"},{"name":"Status","type":"select","options":[{"name":"Todo"},{"name":"Done"}]}]'`,
		"Before using --fields, read lark-base-field-json.md or rely on the same field JSON shape used by +field-create; do not invent field properties.",
		"If --table-name and --fields are both omitted, Base creates one initial table with the platform default schema.",
		"If created as bot, output may include permission_grant; report it so the user knows whether they can open the new Base.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateBaseCreate(runtime)
	},
	DryRun: dryRunBaseCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseCreate(runtime)
	},
}
