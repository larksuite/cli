// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordShareLinkCreate = common.Shortcut{
	Service:     "base",
	Command:     "+record-share-link-create",
	Description: "Generate share links for one or more records (max 100 per request)",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "record-ids", Type: "string_slice", Desc: "record IDs to generate share links for (comma-separated or repeatable, max 100)", Required: true},
	},
	Tips: []string{
		`Single record:  --base-token xxx --table-id tblxxx --record-ids recxxx`,
		`Multiple records: --base-token xxx --table-id tblxxx --record-ids rec001,rec002,rec003`,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordShareBatch(runtime)
	},
	DryRun: dryRunRecordShareBatch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordShareBatch(runtime)
	},
}
