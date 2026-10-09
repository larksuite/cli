// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin || linux

package keylesshelper

import (
	"os"
	"syscall"
)

// Refuse a final symlink swap and avoid blocking if a regular file becomes a FIFO.
const privateKeyOpenFlags = os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
