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
	Risk:        "high-risk-write",
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
		baseHighRiskYesTip,
		`Example text: lark-cli base +field-update --base-token <base_token> --table-id <table_id> --field-id "Status" --json '{"name":"Status","type":"text"}' --yes`,
		`Example select: lark-cli base +field-update --base-token <base_token> --table-id <table_id> --field-id "Status" --json '{"name":"Status","type":"select","multiple":false,"options":[{"name":"Todo"},{"name":"Done"}]}' --yes`,
		`Example auto_number update: lark-cli base +field-update --base-token <base_token> --table-id <table_id> --field-id "编号" --json '{"name":"编号","type":"auto_number","style":{"rules":[{"type":"text","text":"TASK-"},{"type":"created_time","date_format":"yyyyMM"},{"type":"text","text":"-"},{"type":"incremental_number","length":4}]}}' --yes`,
		"Update uses full field-definition PUT semantics. Read the current field first with +field-get, then send the target state.",
		`When --json.type is "auto_number", updating the numbering rules also reapplies them to existing numbers; just submit the target field definition and do not add extra low-level parameters.`,
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
