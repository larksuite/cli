// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package source is a pluggable event source abstraction (separate package to keep
// business registrations free of SDK transitive deps).
package source

import (
	"context"
	"sync"

	"github.com/larksuite/cli/internal/event"
)

// StatusNotifier surfaces SourceState* lifecycle states; detail is free-form context.
type StatusNotifier func(state, detail string)

// Source produces events; emit MUST return quickly (anything slow stalls the SDK read loop).
type Source interface {
	Name() string
	Start(ctx context.Context, eventTypes []string, emit func(*event.RawEvent), notify StatusNotifier) error
}

var (
	registry   []Source
	registryMu sync.Mutex
)

func Register(s Source) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = append(registry, s)
}

func All() []Source {
	registryMu.Lock()
	defer registryMu.Unlock()
	out := make([]Source, len(registry))
	copy(out, registry)
	return out
}

func ResetForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = nil
}
