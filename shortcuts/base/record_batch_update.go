// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordBatchUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+record-batch-update",
	Description: "Batch update records",
	Risk:        "write",
	Scopes:      []string{"base:record:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: `batch update JSON object, e.g. {"record_id_list":["rec_xxx"],"patch":{"Status":"Done"}}; same patch applies to all records`, Required: true},
	},
	Tips: append([]string{
		"Happy path fields: record_id_list is the target record IDs; patch is a field map applied unchanged to every target record.",
		"Do not use +record-batch-update for per-row different values; call +record-upsert per record or use another supported flow.",
		"Before writing, use +field-list to confirm real writable fields; do not write system fields, formula, lookup, or attachment fields as normal CellValue.",
		"Batch update supports max 200 records per call; use the record-batch-update guide for command limits and edge cases.",
	}, recordCellValueHappyPathTips...),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordJSON(runtime)
	},
	DryRun: dryRunRecordBatchUpdate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordBatchUpdate(runtime)
	},
}
