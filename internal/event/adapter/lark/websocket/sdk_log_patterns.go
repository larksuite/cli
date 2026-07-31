// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package websocket

// DO NOT trim trailing spaces — the HasPrefix disambiguator depends on them.
const (
	sdkLogReconnecting = "trying to reconnect"
	sdkLogConnected    = "connected to "
	sdkLogDisconnected = "disconnected to "
)
