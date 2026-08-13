// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package keylessprovider

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewOpenClawInspectCommand_WindowsBatchUsesSystemCmdWithoutPathInterpolation(t *testing.T) {
	launcher := `C:\Users\A B&(team)%literal%\openclaw.cmd`
	cmd, err := newOpenClawInspectCommand(context.Background(), launcher, []string{"PATH=C:\\Windows\\System32"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(filepath.Base(cmd.Path), "cmd.exe") {
		t.Fatalf("command path = %q, want System32 cmd.exe", cmd.Path)
	}
	if cmd.SysProcAttr == nil || !strings.Contains(cmd.SysProcAttr.CmdLine, "/d /q /s /v:off /c") {
		t.Fatalf("cmd line = %#v", cmd.SysProcAttr)
	}
	if strings.Contains(cmd.SysProcAttr.CmdLine, launcher) {
		t.Fatalf("launcher path was interpolated into cmd syntax: %q", cmd.SysProcAttr.CmdLine)
	}
	if !strings.Contains(strings.Join(cmd.Env, "\n"), inspectLauncherEnv+"="+launcher) {
		t.Fatalf("launcher environment missing: %q", cmd.Env)
	}
}

func TestNewOpenClawInspectCommand_WindowsNativeExecutableIsDirect(t *testing.T) {
	launcher := `C:\Program Files\OpenClaw\openclaw.exe`
	cmd, err := newOpenClawInspectCommand(context.Background(), launcher, []string{"TEMP=C:\\Temp"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != launcher || cmd.SysProcAttr != nil {
		t.Fatalf("native command = path %q sys %#v", cmd.Path, cmd.SysProcAttr)
	}
	wantArgs := []string{launcher, "plugins", "inspect", pluginID, "--json"}
	if strings.Join(cmd.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", cmd.Args, wantArgs)
	}
}

func TestNewOpenClawInspectCommand_WindowsRejectsUnknownLauncher(t *testing.T) {
	if _, err := newOpenClawInspectCommand(context.Background(), `C:\\OpenClaw\\openclaw.ps1`, nil); err == nil {
		t.Fatal("PowerShell launcher unexpectedly accepted")
	}
}
