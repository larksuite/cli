// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// BotOptions holds inputs for the bot command.
type BotOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdBot creates the bot command.
func NewCmdBot(f *cmdutil.Factory) *cobra.Command {
	opts := &BotOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "bot",
		Short: "Claude Code Bot: integrate Lark with Claude Code for AI-powered conversations",
		Long: `Claude Code Bot - 飞书 Bot 集成 Claude Code

通过飞书消息与 Claude Code 对话，支持：
- 自然语言对话
- 多轮会话（session 保持）
- 命令模式（/run, /deploy, /status）
- 文件操作
- 多用户支持

示例：
  # 启动 Bot
  lark-cli bot start

  # 使用配置文件启动
  lark-cli bot start --config ~/.lark-cli/bot-config.yaml

  # 后台运行
  lark-cli bot start --daemon

  # 查看状态
  lark-cli bot status

  # 停止 Bot
  lark-cli bot stop`,
	}

	cmd.AddCommand(newCmdBotStart(opts))
	cmd.AddCommand(newCmdBotStatus(&BotStatusOptions{Factory: opts.Factory}))
	cmd.AddCommand(newCmdBotStop(&BotStopOptions{Factory: opts.Factory}))

	return cmd
}
