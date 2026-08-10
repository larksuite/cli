// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldButtonBind = common.Shortcut{
	Service:     "base",
	Command:     "+field-button-bind",
	Description: "Bind or rebind a button field to a workflow",
	Risk:        "write",
	Scopes:      []string{"base:field:update", "base:workflow:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
		{Name: "workflow-id", Desc: "workflow ID returned by +workflow-create or +workflow-list (wkf... prefix)", Required: true},
	},
	Tips: []string{
		`Example: lark-cli base +field-button-bind --base-token <base_token> --table-id <table_id> --field-id <field_id> --workflow-id <wkf_id>`,
		"Create the workflow first with +workflow-create; new workflows are disabled until +workflow-enable is called.",
		"Create the button field without a workflow ID in its property, then bind it with this command.",
		"workflow-id must be the wkf... OpenAPI ID. Internal numeric workflow IDs are not accepted or displayed.",
		"Binding is the source of truth; do not use property.trigger.config.id to create or update a binding.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateWorkflowIDFlag(runtime)
	},
	DryRun: dryRunFieldButtonBind,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldButtonBind(runtime)
	},
}
