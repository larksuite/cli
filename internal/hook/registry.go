// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook

import (
	"context"
	"sync"

	"github.com/larksuite/cli/extension/platform"
)

// ObserverEntry stores one Observer registration. The full hook name
// (already namespaced with plugin prefix by the caller) lets diagnostic
// output point at the responsible plugin.
type ObserverEntry struct {
	Name     string
	When     platform.When
	Selector platform.Selector
	Fn       platform.Observer
}

// WrapperEntry stores one Wrapper registration. Wrappers compose in
// registration order; the outermost (registered first) runs first.
type WrapperEntry struct {
	Name     string
	Selector platform.Selector
	Fn       platform.Wrapper
}

// LifecycleEntry stores one lifecycle handler. Selector is unused
// (lifecycle events are global), but Name is preserved for diagnostics.
type LifecycleEntry struct {
	Name  string
	Event platform.LifecycleEvent
	Fn    platform.LifecycleHandler
}

// Registry holds all registered hooks. The framework constructs one
// Registry per binary execution; concurrent reads after Install
// commits are safe because the maps are not mutated thereafter. Writes
// (during Install) are serialised by the internalplatform.
type Registry struct {
	mu sync.RWMutex

	observers  []ObserverEntry
	wrappers   []WrapperEntry
	lifecycles []LifecycleEntry
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{} }

// Observers returns a snapshot of all registered observers. Order is
// registration order. Diagnostic commands (config plugins show) call
// this to enumerate every hook attached to the binary.
func (r *Registry) Observers() []ObserverEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ObserverEntry, len(r.observers))
	copy(out, r.observers)
	return out
}

// Wrappers returns a snapshot of all registered wrappers. Order is
// registration order (outermost first).
func (r *Registry) Wrappers() []WrapperEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]WrapperEntry, len(r.wrappers))
	copy(out, r.wrappers)
	return out
}

// Lifecycles returns a snapshot of all registered lifecycle handlers.
func (r *Registry) Lifecycles() []LifecycleEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]LifecycleEntry, len(r.lifecycles))
	copy(out, r.lifecycles)
	return out
}

// AddObserver registers an Observer. Caller is responsible for namespacing
// (the platformhost does this). Nil fn is silently skipped -- the staging
// Registrar should reject invalid registrations before this layer.
func (r *Registry) AddObserver(e ObserverEntry) {
	if e.Fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observers = append(r.observers, e)
}

// AddWrapper registers a Wrapper.
func (r *Registry) AddWrapper(e WrapperEntry) {
	if e.Fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wrappers = append(r.wrappers, e)
}

// AddLifecycle registers a LifecycleHandler.
func (r *Registry) AddLifecycle(e LifecycleEntry) {
	if e.Fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lifecycles = append(r.lifecycles, e)
}

// MatchingObservers returns the observers whose selector matches the
// command at the given When stage. Result is a slice (not a generator)
// so callers can iterate without holding the registry lock.
func (r *Registry) MatchingObservers(cmd platform.CommandView, when platform.When) []ObserverEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ObserverEntry, 0, len(r.observers))
	for _, e := range r.observers {
		if e.When == when && e.Selector != nil && e.Selector(cmd) {
			out = append(out, e)
		}
	}
	return out
}

// MatchingWrappers returns the wrappers whose selector matches the
// command. Order matches registration order.
func (r *Registry) MatchingWrappers(cmd platform.CommandView) []WrapperEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]WrapperEntry, 0, len(r.wrappers))
	for _, e := range r.wrappers {
		if e.Selector != nil && e.Selector(cmd) {
			out = append(out, e)
		}
	}
	return out
}

// LifecycleHandlers returns handlers for a given event in registration
// order.
func (r *Registry) LifecycleHandlers(event platform.LifecycleEvent) []LifecycleEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]LifecycleEntry, 0, len(r.lifecycles))
	for _, e := range r.lifecycles {
		if e.Event == event {
			out = append(out, e)
		}
	}
	return out
}

// ComposeWrappers folds a slice of Wrappers into a single Wrapper that
// applies them in registration order (outermost first). Empty slice
// returns the identity Wrapper (next as-is). Inspired by
// grpc.ChainUnaryInterceptor.
func ComposeWrappers(ws []platform.Wrapper) platform.Wrapper {
	if len(ws) == 0 {
		return identityWrapper
	}
	return func(next platform.Handler) platform.Handler {
		// Build from the inside out so the first registered Wrapper
		// ends up outermost.
		for i := len(ws) - 1; i >= 0; i-- {
			next = ws[i](next)
		}
		return next
	}
}

// identityWrapper is the no-op wrapper used when there are no matching
// Wrappers for a command -- callers can always compose into
// next(ctx, inv) without a nil check.
func identityWrapper(next platform.Handler) platform.Handler {
	return func(ctx context.Context, inv platform.Invocation) error {
		return next(ctx, inv)
	}
}
