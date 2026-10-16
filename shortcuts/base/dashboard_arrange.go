// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// BaseDashboardArrange 已被 +dashboard-layout 取代，只为兼容保留：
// golang 代码与 skill 跨仓库发布，旧版 skill 仍会调用本命令，直接删除会让 agent 执行报错。
// 因此保留可执行、但从 --help 隐藏，skill 中也不再引导使用。
var BaseDashboardArrange = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-arrange",
	Description: "Auto-arrange dashboard blocks layout (server-side smart layout)",
	Risk:        "write",
	Scopes:      []string{"base:dashboard:update"},
	AuthTypes:   authTypes(),
	Hidden:      true, // hidden from --help; prefer +dashboard-layout
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		{Name: "user-id-type", Desc: "user ID type: open_id / union_id / user_id"},
	},
	Tips: []string{
		"Server-side smart layout is not deterministic or position-specific; use only when the user asks to arrange or beautify a dashboard, or to tidy up a dashboard created from scratch in this session.",
		"For exact placement, set --position on +dashboard-block-create / +dashboard-block-update instead; do not run this command as a substitute for coordinates the user asked for.",
	},
	DryRun: dryRunDashboardArrange,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardArrange(runtime)
	},
}
