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
		`Example: lark-cli base +record-share-link-create --base-token <base_token> --table-id <table_id> --record-ids <record_id>`,
		"Max 100 record IDs per call; duplicate IDs are ignored.",
		"Output record_share_links maps record_id to URL; records without permission or missing records may be absent.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordShareBatch(runtime)
	},
	DryRun: dryRunRecordShareBatch,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordShareBatch(runtime)
	},
}
