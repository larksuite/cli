// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowButtonFields = common.Shortcut{
	Service:     "base",
	Command:     "+workflow-button-fields",
	Description: "List button fields bound to a workflow",
	Risk:        "read",
	Scopes:      []string{"base:field:read", "base:workflow:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "workflow-id", Desc: "workflow ID (wkf... prefix)", Required: true},
	},
	Tips: []string{
		`Example: lark-cli base +workflow-button-fields --base-token <base_token> --workflow-id <wkf_id>`,
		"workflow-id must be a wkf... OpenAPI ID; internal numeric workflow IDs are not accepted or displayed.",
		"Returns table_id and field_id pairs bound through the button workflow relation.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateWorkflowIDFlag(runtime)
	},
	DryRun: dryRunWorkflowButtonFields,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeWorkflowButtonFields(runtime)
	},
}
