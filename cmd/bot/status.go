// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"fmt"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// BotStatusOptions holds inputs for the bot status command.
type BotStatusOptions struct {
	Factory *cmdutil.Factory
	Ctx     interface{} // Placeholder, not used yet
}

// newCmdBotStatus creates the bot status command.
func newCmdBotStatus(opts *BotStatusOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "查看 Bot 运行状态",
		Long:  "查看 Claude Code Bot 的运行状态、会话数、消息处理统计等",
		RunE: func(cmd *cobra.Command, args []string) error {
			return botStatusRun(opts)
		},
	}

	return cmd
}

// botStatusRun executes the bot status command.
func botStatusRun(opts *BotStatusOptions) error {
	f := opts.Factory

	// TODO: 实现状态检查逻辑
	// 1. 读取 PID 文件
	// 2. 检查进程是否运行
	// 3. 读取 session 统计
	// 4. 读取消息处理统计

	fmt.Fprintf(f.IOStreams.Out, "=== Bot 状态 ===\n")

	// 临时实现
	result := map[string]interface{}{
		"status": "not_implemented",
		"message": "Bot 状态检查功能正在开发中",
	}
	output.PrintJson(f.IOStreams.Out, result)

	return nil
}
