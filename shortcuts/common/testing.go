// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"

	"github.com/spf13/cobra"

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
