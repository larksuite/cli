// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkspaceEntityList = common.Shortcut{
	Service:     "base",
	Command:     "+workspace-entity-list",
	Description: "List bases and BaseApps in a workspace",
	Risk:        "read",
	Scopes:      []string{"base:workspace:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		workspaceTokenFlag(true),
		{Name: "type", Desc: "filter by entity type: base|baseapp; omit to list both", Enum: entityTypeValues},
		{Name: "page-size", Type: "int", Default: "100", Desc: "page size, range 1-100"},
		{Name: "page-token", Desc: "pagination token"},
	},
	Tips: []string{
		"lark-cli base +workspace-entity-list --workspace-token <workspace_token> --type baseapp",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := common.ValidatePageSizeTyped(runtime, "page-size", 100, 1, 100); err != nil {
			return err
		}
		_, err := normalizeEntityType(runtime.Str("type"))
		return err
	},
	DryRun: dryRunWorkspaceEntityList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeWorkspaceEntityList(runtime)
	},
}
