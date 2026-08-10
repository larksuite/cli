// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldButtonUnbind = common.Shortcut{
	Service:     "base",
	Command:     "+field-button-unbind",
	Description: "Unbind a button field from its workflow",
	Risk:        "write",
	Scopes:      []string{"base:field:update", "base:workflow:update"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true), tableRefFlag(true), fieldRefFlag(true)},
	Tips: []string{
		`Example: lark-cli base +field-button-unbind --base-token <base_token> --table-id <table_id> --field-id <field_id>`,
		"Unbind is idempotent; repeating it should still return success when the server has no binding to delete.",
		"Unbinding does not delete the field and does not disable or delete the workflow.",
	},
	DryRun: dryRunFieldButtonUnbind,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldButtonUnbind(runtime)
	},
}
