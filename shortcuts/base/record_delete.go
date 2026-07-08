// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordDelete = common.Shortcut{
	Service:     "base",
	Command:     "+record-delete",
	Description: "Delete one or more records by ID",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:record:delete"},
	AuthTypes:   authTypes(),
	Flags: appendDeleteApprovalFlags(
		baseTokenFlag(true),
		tableRefFlag(true),
		common.Flag{Name: "record-id", Type: "string_array", Desc: "record ID (repeatable)"},
		common.Flag{Name: "json", Desc: `JSON object with record_id_list, e.g. {"record_id_list":["rec_xxx"]}`},
	),
	Tips: []string{
		baseHighRiskYesTip,
		`Example: lark-cli base +record-delete --base-token <base_token> --table-id <table_id> --record-id <record_id_1> --record-id <record_id_2> --yes`,
		"Use --prepare-approval to create the approval URL, or pass --auth-code to execute the delete.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordSelection(runtime)
	},
	DryRun: dryRunRecordDelete,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordDelete(runtime)
	},
}
