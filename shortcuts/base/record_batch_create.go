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
		{Name: "json", Desc: "batch create JSON object", Required: true},
	},
	Tips: []string{
		`Example: --json '{"fields":["Title","Status"],"rows":[["Task A","Open"],["Task B","Done"]]}'`,
		"Agent hint: use the lark-base skill's record-batch-create guide for usage and limits.",
	},
	DryRun: dryRunRecordBatchCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordBatchCreate(runtime)
	},
}
