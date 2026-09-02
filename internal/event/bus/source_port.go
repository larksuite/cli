// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"context"

	"github.com/larksuite/cli/internal/event"
)

// StatusNotifier surfaces source lifecycle states; detail is free-form
// context. It is a function alias (not a named type) so implementations
// satisfy the Source interface without importing this package.
type StatusNotifier = func(state, detail string)

// Source produces events for the bus; emit MUST return quickly (anything slow
// stalls the source's read loop). The bus owns this port and the composition
// root injects implementations — the daemon never constructs one itself.
type Source interface {
	Name() string
	Start(ctx context.Context, eventTypes []string, emit func(*event.RawEvent), notify StatusNotifier) error
}

// Source lifecycle states. The wire values mirror the IPC frame constants —
// the bus forwards them verbatim into source_status frames; a pinning test
// keeps the two vocabularies equal.
const (
	SourceStateConnecting   = "connecting"
	SourceStateConnected    = "connected"
	SourceStateDisconnected = "disconnected"
	SourceStateReconnecting = "reconnecting"
)
