// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestTestNewRuntimeContextForAPIWiresFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{AppID: "self-test-app", AppSecret: "secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "testing-helper"}

	ctx := context.Background()
	rctx := TestNewRuntimeContextForAPI(ctx, cmd, cfg, f, core.AsBot)
	if rctx == nil {
		t.Fatal("TestNewRuntimeContextForAPI returned nil")
	}
	if rctx.Cmd != cmd {
		t.Errorf("Cmd not wired")
	}
	if rctx.Config != cfg {
		t.Errorf("Config not wired")
	}
	if rctx.Factory != f {
		t.Errorf("Factory not wired")
	}
	if !rctx.resolvedAs.IsBot() {
		t.Errorf("resolvedAs not set to bot, got %q", rctx.resolvedAs)
	}
	if rctx.Ctx() != ctx {
		t.Errorf("ctx not wired")
	}

	// User identity should also be accepted — the whole reason for making
	// the parameter explicit is to let user-identity code paths use this
	// helper instead of forking a second one.
	userRctx := TestNewRuntimeContextForAPI(ctx, cmd, cfg, f, core.AsUser)
	if userRctx.resolvedAs != core.AsUser {
		t.Errorf("resolvedAs AsUser not preserved, got %q", userRctx.resolvedAs)
	}
}
