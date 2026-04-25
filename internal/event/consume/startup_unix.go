// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package consume

import (
	"os/exec"
	"syscall"
)

// applyDetachAttrs: Setsid prevents SIGHUP-on-shell-exit from killing the bus.
func applyDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
