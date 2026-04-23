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
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "record-id", Type: "string_array", Desc: "record ID (repeatable)"},
		{Name: "json", Desc: `JSON object with record_id_list, e.g. {"record_id_list":["rec_xxx"]}`},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordSelection(runtime)
	},
	DryRun: dryRunRecordDelete,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordDelete(runtime)
	},
}
