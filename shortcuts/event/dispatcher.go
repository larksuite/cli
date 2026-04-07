// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "context"

// Dispatcher routes normalized events to registered handlers.
type Dispatcher struct {
	registry *HandlerRegistry
}

// NewDispatcher creates a dispatcher backed by the provided registry.
func NewDispatcher(registry *HandlerRegistry) *Dispatcher {
	if registry == nil {
		registry = NewHandlerRegistry()
	}
	return &Dispatcher{registry: registry}
}

// Dispatch runs matching event handlers first, then matching domain handlers.
// Fallback is only used when no direct handlers matched.
func (d *Dispatcher) Dispatch(ctx context.Context, evt *Event) DispatchResult {
	if d == nil || d.registry == nil || evt == nil {
		return DispatchResult{}
	}

	matched := append([]EventHandler{}, d.registry.EventHandlers(evt.EventType)...)
	matched = append(matched, d.registry.DomainHandlers(evt.Domain)...)
	if len(matched) == 0 {
		if fallback := d.registry.FallbackHandler(); fallback != nil {
			matched = append(matched, fallback)
		}
	}

	result := DispatchResult{Results: make([]DispatchRecord, 0, len(matched))}
	for _, handler := range matched {
		handlerResult := handler.Handle(ctx, evt)
		result.Results = append(result.Results, DispatchRecord{
			HandlerID: handler.ID(),
			Status:    handlerResult.Status,
			Reason:    handlerResult.Reason,
			Err:       handlerResult.Err,
			Output:    handlerResult.Output,
		})
	}
	return result
}
