// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseAppBlockGet = common.Shortcut{
	Service:     "base",
	Command:     "+app-block-get",
	Description: "Get a BaseApp page block by ID",
	Risk:        "read",
	Scopes:      []string{"base:appmode_block:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		appTokenFlag(true),
		pageIDFlag(true),
		appBlockIDFlag(true),
	},
	Tips: []string{
		"lark-cli base +app-block-get --app-token <app_token> --page-id <page_id> --block-id <block_id>",
		"Do not call this command for a component whose +app-block-list result has type=unsupported; the API will return an error.",
		"Returns WidgetDetail: widget_id, name, type, optional chart_token/list sub_type, and data_config.",
		"For a text block, the Markdown content is in data_config.text — read it here; text has no +app-block-get-data endpoint.",
		"For a chart's computed result, pass its chart_token to +app-block-get-data --block-id together with --app-token and --base-token.",
		"Read the current data_config here before replacing nested values with +app-block-update.",
	},
	DryRun: dryRunAppBlockGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeAppBlockGet(runtime)
	},
}
