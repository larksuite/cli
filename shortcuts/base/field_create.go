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
	Description: "Create one or more fields",
	Risk:        "write",
	Scopes:      []string{"base:field:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: "field property JSON object or non-empty array of field objects; supports @file", Required: true},
		{Name: "i-have-read-guide", Type: "bool", Desc: "set only after you have read the formula/lookup guide for those field types", Hidden: true},
	},
	Tips: []string{
		`Example text: lark-cli base +field-create --base-token <base_token> --table-id <table_id> --json '{"name":"Status","type":"text"}'`,
		`Example select: lark-cli base +field-create --base-token <base_token> --table-id <table_id> --json '{"name":"Status","type":"select","multiple":false,"options":[{"name":"Todo"},{"name":"Done"}]}'`,
		`+field-create defines storage schema only: choose a documented field type from explicit stored-value requirements and the user's semantics. Treat the field name or business purpose only as a clue to confirm; do not use it to invent derived behavior. Use style only to format the chosen type.`,
		`For explicitly requested derived, automatic, synchronized, or backfilled behavior, use documented formula, lookup, link, workflow, or automation only. If unsupported, do not probe code/web/OpenAPI, create a storage placeholder, or claim completion; report the boundary and alternatives.`,
		"Agent hint: for multiple fields in one table, prefer one array; array items are created sequentially.",
		"For generated arrays, prefer --json @file or an argv-safe subprocess call; do not double-escape JSON inside shell command substitution.",
		`For large arrays, bound successful stdout with --jq 'if .ok then (.data | {created,total,field_get_recommended,next_step,verification_hint}) else . end'; this preserves the full partial-failure envelope. Omit the projection when individual field IDs are needed.`,
		"On successful simple fields, next_step:done means stop: do not list/get fields unless readback is explicitly requested; if needed, filter +field-list with --jq instead of printing every field.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFieldCreate(runtime)
	},
	DryRun: dryRunFieldCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldCreate(runtime)
	},
}
