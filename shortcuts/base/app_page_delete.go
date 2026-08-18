// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseAppPageDelete = common.Shortcut{
	Service:     "base",
	Command:     "+app-page-delete",
	Description: "Delete a BaseApp page",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:appmode_page:delete"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		appTokenFlag(true),
		pageIDFlag(true),
	},
	Tips: []string{
		"lark-cli base +app-page-delete --app-token <app_token> --page-id <page_id> --yes",
		"Deleting a page also deletes its blocks and cannot be recovered; the base data behind the blocks is untouched.",
		baseHighRiskYesTip,
	},
	DryRun: dryRunBaseappPageDelete,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseappPageDelete(runtime)
	},
}
