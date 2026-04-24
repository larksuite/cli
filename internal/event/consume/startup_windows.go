// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package consume

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// applyDetachAttrs sets Windows-specific SysProcAttr so the forked bus
// daemon runs as a detached background process:
//
//   - DETACHED_PROCESS: no console is attached; the child does not
//     inherit the parent's console (equivalent in spirit to Unix Setsid's
//     "escape the controlling terminal").
//   - CREATE_NEW_PROCESS_GROUP: the child lives in its own process group,
//     so a Ctrl+C in the parent shell does not propagate to the bus.
//   - HideWindow: belt-and-suspenders against any GUI/console window
//     flashing on start (DETACHED_PROCESS should already ensure no
//     window, but some Go stdlib paths still probe this flag).
//
// Windows has no fork(2); exec.Start with these flags is the standard
// "daemonize" idiom on this platform.
func applyDetachAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
