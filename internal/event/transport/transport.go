// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package transport abstracts the platform-specific IPC mechanism the
// bus and consume client use to talk to each other: Unix domain sockets
// on POSIX, named pipes on Windows.
package transport

import "net"

// IPC abstracts the platform-specific IPC mechanism (Unix socket on
// POSIX, Named Pipe on Windows). Named `IPC` (not `Transport`) to avoid
// the `transport.Transport` stutter at call sites.
type IPC interface {
	Listen(addr string) (net.Listener, error)
	Dial(addr string) (net.Conn, error)
	Address(appID string) string
	Cleanup(addr string)
}

// New returns the platform-appropriate IPC implementation.
// Implemented in transport_unix.go and transport_windows.go.
