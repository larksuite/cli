// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseAppPageGet = common.Shortcut{
	Service:     "base",
	Command:     "+app-page-get",
	Description: "Get a BaseApp page by ID",
	Risk:        "read",
	Scopes:      []string{"base:appmode_page:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		appTokenFlag(true),
		pageIDFlag(true),
	},
	Tips: []string{
		"lark-cli base +app-page-get --app-token <app_token> --page-id <page_id>",
		"The response is PageDetail and always includes widget summaries. Use +app-block-list when you need paginated widget details.",
	},
	DryRun: dryRunBaseappPageGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseappPageGet(runtime)
	},
}
