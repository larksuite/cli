// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseAppPageCreate = common.Shortcut{
	Service:     "base",
	Command:     "+app-page-create",
	Description: "Create a page in a BaseApp",
	Risk:        "write",
	Scopes:      []string{"base:appmode_page:create", "base:appmode_page:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		appTokenFlag(true),
		{Name: "name", Desc: "page name", Required: true},
	},
	Tips: []string{
		`lark-cli base +app-page-create --app-token <app_token> --name "Overview"`,
		"Page names must be unique within an app; the CLI checks existing pages before creation.",
		"Record the returned page_id; every +app-block-* command needs it.",
		"This release creates top-level pages only; PageGroup placement is not supported.",
	},
	DryRun: dryRunBaseappPageCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseappPageCreate(runtime)
	},
}
