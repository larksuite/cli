// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordBatchCreate = common.Shortcut{
	Service:     "base",
	Command:     "+record-batch-create",
	Description: "Batch create records",
	Risk:        "write",
	Scopes:      []string{"base:record:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: `batch create JSON object; create_records contains one field map per record, e.g. {"create_records":[{"Name":"Task A","Status":"Todo"},{"Name":"Task B","Score":20}]}`, Required: true},
	},
	Tips: append([]string{
		"Happy path field: create_records is an array of independent record field maps.",
		`Example: {"create_records":[{"Name":"Task A","Status":"Todo"},{"Name":"Task B","Score":20}]}.`,
		"Before writing, use +field-list to confirm real writable fields; do not write system fields, formula, lookup, or attachment fields as normal CellValue.",
		"Batch create supports max 200 records per call.",
		"After batch-creating known helper rows, use the returned record IDs and your submitted rows; do not immediately +record-list the same table unless you need server-normalized formula/lookup values or failure diagnosis.",
		"Use the record-batch-create guide for command limits and edge cases.",
	}, recordCellValueHappyPathTips...),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordJSON(runtime)
	},
	DryRun: dryRunRecordBatchCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordBatchCreate(runtime)
	},
}
