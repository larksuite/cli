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
	Description: "Batch update records with shared or per-record fields",
	Risk:        "write",
	Scopes:      []string{"base:record:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: `batch update JSON object; use {"record_id_list":["rec_xxx"],"patch":{"Status":"Done"}} for one shared patch, or {"update_records":{"rec_xxx":{"Status":"Done"}}} for per-record values`, Required: true},
	},
	Tips: append([]string{
		"Two mutually exclusive modes are supported: record_id_list plus patch applies one field map to every target; update_records maps each record ID to its own field map.",
		`Per-record example: {"update_records":{"recA":{"Status":["Done"]},"recB":{"Score":20}}}.`,
		"Per-record mode returns only optional ignored_fields and does not check whether record IDs exist; read records back when confirmation is required.",
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
