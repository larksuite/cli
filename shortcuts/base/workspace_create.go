// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkspaceCreate = common.Shortcut{
	Service:     "base",
	Command:     "+workspace-create",
	Description: "Create a workspace",
	Risk:        "write",
	Scopes:      []string{"base:workspace:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		{Name: "name", Desc: "workspace name", Required: true},
	},
	Tips: []string{
		`lark-cli base +workspace-create --name "Growth team"`,
		"Record the returned workspace_token and url; +workspace-entity-list, +workspace-move-in, and +app-create need the token.",
	},
	DryRun: dryRunWorkspaceCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeWorkspaceCreate(runtime)
	},
}
