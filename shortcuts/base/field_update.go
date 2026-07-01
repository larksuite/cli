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
		{Name: "reformat-existing-records", Type: "bool", Desc: "for auto_number updates only: regenerate existing values with the updated numbering rules"},
		{Name: "i-have-read-guide", Type: "bool", Desc: "acknowledge reading formula/lookup guide before creating or updating those field types", Hidden: true},
	},
	Tips: []string{
		baseHighRiskYesTip,
		`Example text: lark-cli base +field-update --base-token <base_token> --table-id <table_id> --field-id "Status" --json '{"name":"Status","type":"text"}' --yes`,
		`Example select: lark-cli base +field-update --base-token <base_token> --table-id <table_id> --field-id "Status" --json '{"name":"Status","type":"select","multiple":false,"options":[{"name":"Todo"},{"name":"Done"}]}' --yes`,
		`Example auto_number reflow: lark-cli base +field-update --base-token <base_token> --table-id <table_id> --field-id "自动编号" --json '{"name":"自动编号","type":"auto_number","style":{"rules":[{"type":"text","text":"ORD-"},{"type":"created_time","date_format":"yyyyMMdd"},{"type":"text","text":"-"},{"type":"incremental_number","length":4}]}}' --reformat-existing-records --yes`,
		"Update uses full field-definition PUT semantics. Read the current field first with +field-get, then send the target state.",
		"Auto-number updates that must apply new rules to existing rows stay on +field-update; add --reformat-existing-records instead of switching to raw lark-cli api writes.",
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
