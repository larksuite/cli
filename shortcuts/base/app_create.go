// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseAppCreate = common.Shortcut{
	Service:     "base",
	Command:     "+app-create",
	Description: "Create a new BaseApp in a Workspace (not a copy)",
	Risk:        "write",
	Scopes: []string{
		"base:appmode:create",
		"base:workspace:update",
	},
	AuthTypes: authTypes(),
	Flags: []common.Flag{
		{Name: "name", Desc: "BaseApp name", Required: true},
		workspaceTokenFlag(true),
		{Name: "theme-style", Desc: "theme style", Enum: []string{"default", "cloudBlue", "fresh", "softLight", "future", "technology"}},
	},
	Tips: []string{
		`lark-cli base +app-create --name "Sales app" --workspace-token <workspace_token>`,
		`lark-cli base +app-create --name "Sales app" --workspace-token <workspace_token> --theme-style cloudBlue`,
		"This command creates a new empty BaseApp; it does not copy an existing BaseApp or its pages and blocks.",
		"Create or select a Base separately when the app needs data.",
		"Record the returned app_token; page and block commands require it.",
	},
	DryRun: dryRunBaseappCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseappCreate(runtime)
	},
}
