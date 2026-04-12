// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"fmt"
	"os"
	"syscall"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// BotStopOptions holds inputs for the bot stop command.
type BotStopOptions struct {
	Factory *cmdutil.Factory
	Ctx     interface{} // Placeholder, not used yet
}

// newCmdBotStop creates the bot stop command.
func newCmdBotStop(opts *BotStopOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "停止运行中的 Bot",
		Long:  "优雅地停止 Claude Code Bot，保存会话状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			return botStopRun(opts)
		},
	}

	return cmd
}

// botStopRun executes the bot stop command.
func botStopRun(opts *BotStopOptions) error {
	f := opts.Factory

	// TODO: 实现停止逻辑
	// 1. 读取 PID 文件
	// 2. 发送 SIGTERM 信号
	// 3. 等待进程退出
	// 4. 清理 PID 文件

	fmt.Fprintf(f.IOStreams.Out, "=== 停止 Bot ===\n")

	// 临时实现
	result := map[string]interface{}{
		"status": "not_implemented",
		"message": "Bot 停止功能正在开发中",
	}
	output.PrintJson(f.IOStreams.Out, result)

	return nil
}

// findBotProcess 查找运行中的 Bot 进程
func findBotProcess() (int, error) {
	// TODO: 实现 PID 文件读取
	return 0, fmt.Errorf("未找到运行中的 Bot 进程")
}

// stopProcess 停止指定进程
func stopProcess(pid int) error {
	// 发送 SIGTERM 信号（优雅退出）
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}
