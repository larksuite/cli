// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRecordGet = common.Shortcut{
	Service:     "base",
	Command:     "+record-get",
	Description: "Get one or more records by ID",
	Risk:        "read",
	Scopes:      []string{"base:record:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "record-id", Type: "string_array", Desc: "record ID (repeatable)"},
		{Name: "field-id", Type: "string_array", Desc: "field ID or field name to include (repeatable)"},
		{Name: "json", Desc: `JSON object with record_id_list, e.g. {"record_id_list":["rec_xxx"]}`},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateRecordSelection(runtime)
	},
	DryRun: dryRunRecordGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordGet(runtime)
	},
}
