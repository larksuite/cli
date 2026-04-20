// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordSearch = common.Shortcut{
	Service:     "base",
	Command:     "+record-search",
	Description: "Search records in a table",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: "record search JSON object", Required: true},
	},
	Tips: []string{
		`Example: --json '{"keyword":"Alice","search_fields":["Name"]}'`,
		"Agent hint: use the lark-base skill's record-search guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordJSON(runtime)
	},
	DryRun: dryRunRecordSearch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordSearch(runtime)
	},
}
