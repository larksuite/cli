// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseAppPageList = common.Shortcut{
	Service:     "base",
	Command:     "+app-page-list",
	Description: "List pages in a BaseApp",
	Risk:        "read",
	Scopes:      []string{"base:appmode_page:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		appTokenFlag(true),
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size; must be positive"},
		{Name: "page-token", Desc: "pagination token"},
	},
	Tips: []string{
		"lark-cli base +app-page-list --app-token <app_token>",
		"Use the returned page_id for +app-page-get/update/delete and every +app-block-* command.",
		"If a returned page has name=\"\", the current user has no permission to that page; do not treat it as an untitled page or use it as an operation target.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidatePageSizeTyped(runtime, "page-size", 20, 1, int(^uint(0)>>1))
		return err
	},
	DryRun: dryRunBaseappPageList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseappPageList(runtime)
	},
}
