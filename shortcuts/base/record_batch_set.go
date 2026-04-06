// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordBatchSet = common.Shortcut{
	Service:     "base",
	Command:     "+record-batch-set",
	Description: "Batch set records",
	Risk:        "write",
	Scopes:      []string{"base:record:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: "batch set JSON object, passed through as request body, e.g. {\"record_id_list\":[\"rec_xxx\"],\"patch\":{\"field_id_or_name\":\"value\"}}", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordJSON(runtime)
	},
	DryRun: dryRunRecordBatchSet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordBatchSet(runtime)
	},
}
