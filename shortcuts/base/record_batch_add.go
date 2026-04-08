// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordBatchAdd = common.Shortcut{
	Service:     "base",
	Command:     "+record-batch-add",
	Description: "Batch add records",
	Risk:        "write",
	Scopes:      []string{"base:record:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: "batch add JSON object, e.g. {\"fields\":[],\"rows\":[]}", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordJSON(runtime)
	},
	DryRun: dryRunRecordBatchAdd,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordBatchAdd(runtime)
	},
}
