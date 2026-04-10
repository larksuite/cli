// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/bot"
	"github.com/spf13/cobra"
)

// BotStartOptions holds inputs for the bot start command.
type BotStartOptions struct {
	Factory  *cmdutil.Factory
	Ctx      context.Context
	Config   string
	Daemon   bool
}

// newCmdBotStart creates the bot start command.
func newCmdBotStart(opts *BotOptions) *cobra.Command {
	var config string
	var daemon bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "启动 Claude Code Bot",
		Long:  "启动飞书 Bot，监听消息并路由给 Claude Code 处理",
		RunE: func(cmd *cobra.Command, args []string) error {
			startOpts := &BotStartOptions{
				Factory: opts.Factory,
				Ctx:     cmd.Context(),
				Config:  config,
				Daemon:  daemon,
			}
			return botStartRun(startOpts)
		},
	}

	cmd.Flags().StringVar(&config, "config", "", "配置文件路径")
	cmd.Flags().BoolVar(&daemon, "daemon", false, "后台运行模式")

	return cmd
}

// botStartRun executes the bot start command.
func botStartRun(opts *BotStartOptions) error {
	f := opts.Factory
	ctx := opts.Ctx
	io := f.IOStreams

	fmt.Fprintf(io.Out, "=== Claude Code Bot 启动中 ===\n")

	// 1. Validate claude CLI is available
	fmt.Fprintf(io.Out, "验证 Claude Code CLI...\n")
	if err := bot.ValidateClaudeCLI(ctx); err != nil {
		return fmt.Errorf("claude CLI validation failed: %w", err)
	}
	fmt.Fprintf(io.Out, "✓ Claude Code CLI 已就绪\n")

	// 2. Initialize session manager
	fmt.Fprintf(io.Out, "初始化 Session 管理器...\n")
	sessionMgr, err := bot.NewSessionManager(bot.SessionManagerConfig{
		TTL: 24 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("failed to create session manager: %w", err)
	}
	fmt.Fprintf(io.Out, "✓ Session 管理器已初始化\n")

	// 3. Initialize Claude client
	claudeClient := bot.NewClaudeClient(bot.ClaudeClientConfig{
		WorkDir:         "/tmp/lark-claude-bot",
		Timeout:         5 * time.Minute,
		MaxRetries:      3,
		SkipPermissions: true,
	})

	// 4. Initialize bot handler
	fmt.Fprintf(io.Out, "初始化 Bot 处理器...\n")
	botHandler, err := bot.NewBotHandler(bot.BotHandlerConfig{
		ClaudeClient:   claudeClient,
		SessionManager: sessionMgr,
		WorkDir:        "/tmp/lark-claude-bot",
	})
	if err != nil {
		return fmt.Errorf("failed to create bot handler: %w", err)
	}
	fmt.Fprintf(io.Out, "✓ Bot 处理器已初始化\n")

	// 5. Initialize event subscriber
	fmt.Fprintf(io.Out, "初始化事件订阅...\n")

	// Load config
	config, err := core.LoadMultiAppConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return output.ErrWithHint(output.ExitValidation, "config", "not configured", "run: lark-cli config init")
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	app := config.CurrentAppConfig(f.Invocation.Profile)
	if app == nil {
		return output.ErrWithHint(output.ExitValidation, "config", "no active profile", "run: lark-cli profile list")
	}

	subscriber := bot.NewEventSubscriber(bot.EventSubscriberConfig{
		BotHandler: botHandler,
		AppID:      app.AppId,
		AppSecret:  app.AppSecret,
		Brand:      string(app.Brand),
		Quiet:      false,
	})
	fmt.Fprintf(io.Out, "✓ 事件订阅已初始化\n")

	// 6. Start event subscription (blocking)
	fmt.Fprintf(io.Out, "\n=== 开始监听飞书消息 ===\n")

	if err := subscriber.Subscribe(ctx); err != nil {
		return fmt.Errorf("event subscription failed: %w", err)
	}

	fmt.Fprintf(io.Out, "\n=== Bot 已停止 ===\n")
	return nil
}
