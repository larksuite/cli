// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package keylessprovider

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

const inspectLauncherEnv = "LARK_CLI_OPENCLAW_INSPECT_LAUNCHER"

// newOpenClawInspectCommand runs native launchers directly and npm's .cmd/.bat
// launchers through the OS-owned command interpreter. The launcher path travels
// in a dedicated environment variable and is expanded once inside a quoted
// command, while delayed expansion is disabled. This avoids interpolating a
// user-controlled path into cmd.exe syntax.
func newOpenClawInspectCommand(ctx context.Context, executable string, env []string) (*exec.Cmd, error) {
	ext := strings.ToLower(filepath.Ext(executable))
	if ext == ".exe" || ext == ".com" {
		cmd := exec.CommandContext(ctx, executable, "plugins", "inspect", pluginID, "--json")
		cmd.Env = env
		return cmd, nil
	}
	if ext != ".cmd" && ext != ".bat" {
		return nil, fmt.Errorf("unsupported Windows OpenClaw launcher extension %q", ext)
	}

	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, fmt.Errorf("resolve Windows system directory: %w", err)
	}
	commandInterpreter := filepath.Join(systemDir, "cmd.exe")
	cmd := exec.CommandContext(ctx, commandInterpreter)
	cmd.Args = nil
	cmd.Env = append(append([]string(nil), env...), inspectLauncherEnv+"="+executable)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `/d /q /s /v:off /c ""%` + inspectLauncherEnv + `%" plugins inspect ` + pluginID + ` --json"`,
	}
	return cmd, nil
}
