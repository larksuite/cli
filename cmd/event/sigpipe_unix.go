// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build unix

package event

import (
	"os/signal"
	"syscall"
)

// ignoreBrokenPipe stops Go's default SIGPIPE-on-stdout terminate behavior.
// Subsequent stdout writes return syscall.EPIPE so consume can shut down cleanly.
func ignoreBrokenPipe() {
	signal.Ignore(syscall.SIGPIPE)
}
