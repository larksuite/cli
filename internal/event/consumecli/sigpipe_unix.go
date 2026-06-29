// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build unix

package consumecli

import (
	"os/signal"
	"syscall"
)

func ignoreBrokenPipe() {
	signal.Ignore(syscall.SIGPIPE)
}
