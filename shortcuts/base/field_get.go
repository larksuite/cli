// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldGet = common.Shortcut{
	Service:     "base",
	Command:     "+field-get",
	Description: "Get a field by ID or name",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true), tableRefFlag(true), fieldRefFlag(true)},
	Tips: []string{
		`Example: lark-cli base +field-get --base-token <base_token> --table-id <table_id> --field-id "Status"`,
		"field-id accepts a field ID (fld...) or the field name from the current table.",
		"Returns full field configuration; use it as the baseline before +field-update.",
	},
	DryRun: dryRunFieldGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldGet(runtime)
	},
}
