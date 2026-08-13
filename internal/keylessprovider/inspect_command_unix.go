// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin || linux

package keylessprovider

import (
	"context"
	"os/exec"
)

func newOpenClawInspectCommand(ctx context.Context, executable string, env []string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, executable, "plugins", "inspect", pluginID, "--json")
	cmd.Env = env
	return cmd, nil
}
