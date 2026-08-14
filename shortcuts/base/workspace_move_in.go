// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkspaceMoveIn = common.Shortcut{
	Service:     "base",
	Command:     "+workspace-move-in",
	Description: "Move an existing Base or BaseApp into a workspace",
	Risk:        "write",
	Scopes:      []string{"base:workspace:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		workspaceTokenFlag(true),
		{Name: "entity-token", Desc: "base_token or app_token to move into the workspace", Required: true},
	},
	Tips: []string{
		"lark-cli base +workspace-move-in --workspace-token <workspace_token> --entity-token <base_token>",
		"This moves the entity into the workspace tree; it does not create the Base or App.",
		"The current OpenAPI does not accept entity_type or ordering fields for move-in.",
	},
	DryRun: dryRunWorkspaceMoveIn,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeWorkspaceMoveIn(runtime)
	},
}
