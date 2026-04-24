// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package consume

import (
	"os/exec"
	"syscall"
)

// applyDetachAttrs sets Unix-specific SysProcAttr so the forked bus
// daemon detaches from the controlling terminal. Without Setsid, the
// bus would receive SIGHUP when the parent shell exits and die with it.
func applyDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
