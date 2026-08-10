// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldButtonBindingGet = common.Shortcut{
	Service:     "base",
	Command:     "+field-button-binding-get",
	Description: "Get the workflow binding for a button field",
	Risk:        "read",
	Scopes:      []string{"base:field:read", "base:workflow:read"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true), tableRefFlag(true), fieldRefFlag(true)},
	Tips: []string{
		`Example: lark-cli base +field-button-binding-get --base-token <base_token> --table-id <table_id> --field-id <field_id>`,
		"Returns the workflow_id as a wkf... OpenAPI ID when a binding exists.",
		"The binding source of truth is the button workflow relation, not field property.trigger.",
	},
	DryRun: dryRunFieldButtonBindingGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldButtonBindingGet(runtime)
	},
}
