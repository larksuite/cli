// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package consume

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// applyDetachAttrs: Windows daemonize via DETACHED_PROCESS + new process group + HideWindow.
func applyDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
