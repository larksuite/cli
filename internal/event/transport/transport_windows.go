// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

// internal/event/transport/transport_windows.go
//
// Windows Named Pipe transport for the event bus IPC. Uses Microsoft's
// go-winio (same library docker/containerd depend on) so the consumer
// side looks identical to Unix sockets from the bus/consume layer —
// they just work against `net.Conn` / `net.Listener`.
//
// Pipe name format: \\.\pipe\lark-cli-<appID>. The pipe is a kernel
// object (not on the filesystem) so Cleanup is a no-op: Windows
// releases the pipe automatically when the server process exits.

package transport

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// pipeBufferSize is the per-direction kernel buffer for each named pipe.
// 64 KB is generous for our JSON event payloads (typical < 10 KB, bulk
// events ~ 30-50 KB) — one event always fits without forcing a partial
// write, and the writer rarely blocks waiting for the reader. Raising it
// further wastes kernel memory per connection without reducing syscalls;
// lowering it causes more context switches on burst traffic.
const pipeBufferSize = 65536

type windowsTransport struct{}

// New returns a Named Pipe transport for Windows.
func New() IPC {
	return &windowsTransport{}
}

func (t *windowsTransport) Listen(addr string) (net.Listener, error) {
	// PipeConfig values mirror the kernel defaults we care about:
	//   - MessageMode=false → byte stream, matches our newline-delimited JSON protocol
	//   - InputBufferSize / OutputBufferSize match a typical event payload plus slack
	// SecurityDescriptor is left empty → defaults to the creating user,
	// which is exactly what we want (per-user IPC, not system-wide).
	return winio.ListenPipe(addr, &winio.PipeConfig{
		InputBufferSize:  pipeBufferSize,
		OutputBufferSize: pipeBufferSize,
	})
}

func (t *windowsTransport) Dial(addr string) (net.Conn, error) {
	// 5s dial timeout matches the forkBus retry budget on the consume side.
	timeout := 5 * time.Second
	return winio.DialPipe(addr, &timeout)
}

// Address returns the pipe path for a given appID. Named pipe names live
// in a global kernel namespace, keyed by appID so multiple bus daemons
// for different apps coexist without collision. Backslash and NUL are
// stripped defensively — a corrupt AppID should not be able to reshape
// the pipe path.
func (t *windowsTransport) Address(appID string) string {
	return `\\.\pipe\lark-cli-` + sanitizePipeAppID(appID)
}

func sanitizePipeAppID(appID string) string {
	if appID == "" {
		return "_"
	}
	out := make([]rune, 0, len(appID))
	for _, r := range appID {
		if r == '\\' || r == '/' || r == 0 {
			out = append(out, '_')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// Cleanup is a no-op on Windows: named pipes are kernel objects that
// get released automatically when the server-side handle closes. There
// is no stale file to remove like on Unix.
func (t *windowsTransport) Cleanup(addr string) {}
