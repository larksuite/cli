// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseAppGet = common.Shortcut{
	Service:     "base",
	Command:     "+app-get",
	Description: "Get BaseApp info, page summaries, and the referenced Base/Table map",
	Risk:        "read",
	Scopes:      []string{"base:appmode:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		appTokenFlag(true),
	},
	Tips: []string{
		"lark-cli base +app-get --app-token <app_token>",
		"ref maps each Base token currently referenced by app widgets to the names of its referenced tables; table/field/record commands take the Base token keys.",
		"The response includes page summaries. Use +app-page-get or +app-block-list for component details.",
	},
	DryRun: dryRunBaseappGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseappGet(runtime)
	},
}
