// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package transport

import (
	"net"
	"path/filepath"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/vfs"
)

const dialTimeout = 5 * time.Second // matches winio.DialPipe for cross-platform symmetry

type unixTransport struct{}

func New() IPC {
	return &unixTransport{}
}

func (t *unixTransport) Listen(addr string) (net.Listener, error) {
	if err := vfs.MkdirAll(filepath.Dir(addr), 0700); err != nil {
		return nil, err
	}
	return net.Listen("unix", addr)
}

func (t *unixTransport) Dial(addr string) (net.Conn, error) {
	return net.DialTimeout("unix", addr, dialTimeout)
}

// Address: NOT os.UserHomeDir — honours LARKSUITE_CLI_CONFIG_DIR override.
func (t *unixTransport) Address(appID string) string {
	return filepath.Join(core.GetConfigDir(), "events", event.SanitizeAppID(appID), "bus.sock")
}

func (t *unixTransport) Cleanup(addr string) {
	_ = vfs.Remove(addr)
}
