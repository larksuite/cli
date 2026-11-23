// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package transport: Unix sockets on POSIX, named pipes on Windows.
package transport

import "net"

type IPC interface {
	Listen(addr string) (net.Listener, error)
	Dial(addr string) (net.Conn, error)
	Address(appID string) string
	Cleanup(addr string)
}
