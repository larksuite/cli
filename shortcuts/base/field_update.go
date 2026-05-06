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
		{Name: "json", Desc: "complete field definition JSON object; update uses full PUT semantics, not a patch", Required: true},
		{Name: "i-have-read-guide", Type: "bool", Desc: "acknowledge reading formula/lookup guide before creating or updating those field types", Hidden: true},
	},
	Tips: []string{
		`Example: lark-cli base +field-update --base-token <base_token> --table-id <table_id> --field-id <field_id> --json '{"name":"Status","type":"text"}'`,
		`Example: lark-cli base +field-update --base-token <base_token> --table-id <table_id> --field-id <field_id> --json '{"name":"Status","type":"select","multiple":false,"options":[{"name":"Todo"},{"name":"Done"}]}'`,
		"Update uses full field-definition PUT semantics. Read the current field first with +field-get, then send the target state.",
		"Type conversion is allowlist-based: only use CLI for safe conversions; otherwise migrate through a new field, or ask the user to finish high-risk conversions in the web UI.",
		"Formula and lookup updates require reading the corresponding guide first.",
		"Agent hint: use the lark-base skill's field-update guide for JSON shape, type-conversion rules, and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFieldUpdate(runtime)
	},
	DryRun: dryRunFieldUpdate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldUpdate(runtime)
	},
}
