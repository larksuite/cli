// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"sync"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// TestNewRuntimeContext creates a RuntimeContext for testing purposes.
// Only Cmd and Config are set; other fields (Factory, larkSDK, etc.) are nil.
func TestNewRuntimeContext(cmd *cobra.Command, cfg *core.CliConfig) *RuntimeContext {
	return &RuntimeContext{Cmd: cmd, Config: cfg}
}

// TestNewRuntimeContextWithCtx creates a RuntimeContext with an explicit context
// for tests that invoke functions which call Ctx() (e.g. HTTP request helpers).
func TestNewRuntimeContextWithCtx(ctx context.Context, cmd *cobra.Command, cfg *core.CliConfig) *RuntimeContext {
	return &RuntimeContext{ctx: ctx, Cmd: cmd, Config: cfg}
}

// TestNewRuntimeContextWithIdentity creates a RuntimeContext with a specific identity for testing.
func TestNewRuntimeContextWithIdentity(cmd *cobra.Command, cfg *core.CliConfig, as core.Identity) *RuntimeContext {
	return &RuntimeContext{Cmd: cmd, Config: cfg, resolvedAs: as}
}

// TestNewRuntimeContextWithBotInfo creates a RuntimeContext with a pre-set BotInfo for testing.
func TestNewRuntimeContextWithBotInfo(cmd *cobra.Command, cfg *core.CliConfig, info *BotInfo) *RuntimeContext {
	rctx := &RuntimeContext{Cmd: cmd, Config: cfg}
	rctx.botInfoFunc = sync.OnceValues(func() (*BotInfo, error) {
		return info, nil
	})
	return rctx
}

// TestNewRuntimeContextForAPI creates a RuntimeContext ready for HTTP tests:
// sets Cmd, Config, Factory, context, and the requested identity so callers
// can invoke DoAPI / CallAPI directly without wiring through a cobra parent
// command.
//
// Pass core.AsBot or core.AsUser explicitly — exposing the identity as a
// parameter keeps the helper reusable for tests that need to exercise the
// user-identity code path (token store, auth login, etc.) without forking
// into a second near-identical helper.
func TestNewRuntimeContextForAPI(ctx context.Context, cmd *cobra.Command, cfg *core.CliConfig, f *cmdutil.Factory, as core.Identity) *RuntimeContext {
	return &RuntimeContext{
		ctx:        ctx,
		Cmd:        cmd,
		Config:     cfg,
		Factory:    f,
		resolvedAs: as,
	}
}
