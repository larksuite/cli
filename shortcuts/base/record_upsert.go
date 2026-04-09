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
		{Name: "json", Desc: `record JSON object, e.g. {"Name":"Alice"}; read skills/lark-base/references/lark-base-record-upsert.md`, Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordJSON(runtime)
	},
	DryRun: dryRunRecordUpsert,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordUpsert(runtime)
	},
}
