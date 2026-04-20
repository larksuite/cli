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
		{Name: "json", Desc: "batch update JSON object", Required: true},
	},
	Tips: []string{
		`Example: --json '{"record_id_list":["recXXX"],"patch":{"Status":"Done"}}'`,
		"Agent hint: use the lark-base skill's record-batch-update guide for usage and limits.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordJSON(runtime)
	},
	DryRun: dryRunRecordBatchUpdate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordBatchUpdate(runtime)
	},
}
