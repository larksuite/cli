// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordUpsert = common.Shortcut{
	Service:     "base",
	Command:     "+record-upsert",
	Description: "Create or update a record",
	Risk:        "write",
	Scopes:      []string{"base:record:create", "base:record:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		recordRefFlag(false),
		{Name: "json", Desc: `record field map JSON object, e.g. {"Name":"Alice","Status":"Todo"}; do not wrap in fields`, Required: true},
	},
	Tips: append([]string{
		"Happy path JSON is a top-level field map: each key is a real field name or field ID, each value is that field's CellValue.",
		"Without --record-id this creates a record; with --record-id this updates that record. It does not auto-upsert by business key.",
		"Before writing, use +field-list to confirm real writable fields; do not write system fields, formula, lookup, or attachment fields as normal CellValue.",
		"Use the record-upsert guide for command limits and edge cases.",
	}, recordCellValueHappyPathTips...),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordJSON(runtime)
	},
	DryRun: dryRunRecordUpsert,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordUpsert(runtime)
	},
}
