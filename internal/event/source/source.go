// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package source is a pluggable event source abstraction. Sources produce
// RawEvents into the bus; the bus fans them out to consume subscribers.
//
// Keeping this package separate from internal/event means business
// registrations (events/im, events/mail, ...) can import event without
// transitively pulling in SDK dependencies like larkws.
package source

import (
	"context"
	"sync"

	"github.com/larksuite/cli/internal/event"
)

// StatusNotifier is how a source surfaces lifecycle state (connecting /
// connected / disconnected / reconnecting) to the bus for broadcast.
// state values are the SourceState* constants in the protocol package.
// detail is free-form context (e.g. "attempt 1", error summary).
// Sources that have no such lifecycle may never call this.
type StatusNotifier func(state, detail string)

// Source produces events. Implementations block in Start until ctx is
// cancelled or an unrecoverable error occurs.
//
// IMPORTANT emit contract: the emit callback passed to Start MUST return
// quickly (typically microseconds). Hub.Publish dispatches to subscribers
// via drop-oldest non-blocking sends, so emit itself will not block on
// slow consumers — but anything else the source does on the hot path
// (between receiving a message from upstream and calling emit) sits on
// the SDK's read loop. A stalled emit path means the SDK stops reading
// from the upstream socket, the upstream gateway eventually times out
// its write buffer, and the connection gets dropped with a reconnect
// cycle as the only recovery. Do expensive work in a worker goroutine
// that emit hands off to via a channel, never inline.
type Source interface {
	Name() string
	// Start begins producing events. eventTypes is the set of upstream
	// event type names the bus wants subscribed; sources that talk to a
	// server-side dispatcher (e.g. FeishuSource) use this to opt into
	// only the events downstream consumers care about. notify is the
	// optional lifecycle callback (see StatusNotifier).
	Start(ctx context.Context, eventTypes []string, emit func(*event.RawEvent), notify StatusNotifier) error
}

var (
	registry   []Source
	registryMu sync.Mutex
)

// Register adds a source. Called from init() of source implementations
// or from tests that need to inject mocks.
func Register(s Source) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = append(registry, s)
}

// All returns a snapshot of registered sources.
func All() []Source {
	registryMu.Lock()
	defer registryMu.Unlock()
	out := make([]Source, len(registry))
	copy(out, registry)
	return out
}

// ResetForTest clears the registry. Only for tests.
func ResetForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = nil
}
