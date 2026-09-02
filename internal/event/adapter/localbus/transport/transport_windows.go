// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

// Windows Named Pipe transport via go-winio; pipe is a kernel object so Cleanup is a no-op.

package transport

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/larksuite/cli/internal/event"
)

const pipeBufferSize = 65536 // per-direction; one event payload always fits

type windowsTransport struct{}

func New() IPC {
	return &windowsTransport{}
}

func (t *windowsTransport) Listen(addr string) (net.Listener, error) {
	// Empty SecurityDescriptor → per-user IPC (the creating user only).
	return winio.ListenPipe(addr, &winio.PipeConfig{
		InputBufferSize:  pipeBufferSize,
		OutputBufferSize: pipeBufferSize,
	})
}

func (t *windowsTransport) Dial(addr string) (net.Conn, error) {
	timeout := 5 * time.Second
	return winio.DialPipe(addr, &timeout)
}

// Address: SanitizeAppID prevents corrupt AppID from reshaping the pipe path.
func (t *windowsTransport) Address(appID string) string {
	return `\\.\pipe\lark-cli-` + event.SanitizeAppID(appID)
}

func (t *windowsTransport) Cleanup(addr string) {}
