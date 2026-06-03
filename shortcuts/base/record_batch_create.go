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
		{Name: "json", Desc: `batch create JSON object, e.g. {"fields":["Name","Status"],"rows":[["Task A","Todo"],["Task B",null]]}; rows follow fields order`, Required: true},
	},
	Tips: append([]string{
		"Happy path fields: fields is the column order; rows is an array of row arrays; each row must match fields order and may use null for empty cells.",
		"Before writing, use +field-list to confirm real writable fields; do not write system fields, formula, lookup, or attachment fields as normal CellValue.",
		"Batch create supports max 200 rows per call.",
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
