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
	Description: "Batch update records with record-specific fields",
	Risk:        "write",
	Scopes:      []string{"base:record:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: `batch update JSON object; update_records maps each record ID to its field map, e.g. {"update_records":{"recA":{"Status":["Done"]},"recB":{"Score":20}}}`, Required: true},
	},
	Tips: append([]string{
		"Happy path field: update_records maps each record ID to its own field map.",
		`Example: {"update_records":{"recA":{"Status":["Done"]},"recB":{"Score":20}}}.`,
		"The response contains only optional ignored_fields and does not check whether record IDs exist; read records back when confirmation is required.",
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
