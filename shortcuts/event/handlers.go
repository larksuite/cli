// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "context"

// HandlerStatus reports how a handler processed an event.
type HandlerStatus string

const (
	HandlerStatusHandled HandlerStatus = "handled"
	HandlerStatusSkipped HandlerStatus = "skipped"
	HandlerStatusFailed  HandlerStatus = "failed"
)

// HandlerResult captures the outcome from a handler invocation.
type HandlerResult struct {
	Status HandlerStatus
	Reason string
	Err    error
	Output interface{}
}

// DispatchRecord captures the recorded outcome for a single handler.
type DispatchRecord struct {
	HandlerID string
	Status    HandlerStatus
	Reason    string
	Err       error
	Output    interface{}
}

// DispatchResult stores the collected outcomes for all dispatched handlers.
type DispatchResult struct {
	Results []DispatchRecord
}

// EventHandler routes normalized events by event type or domain.
type EventHandler interface {
	ID() string
	EventType() string
	Domain() string
	Handle(ctx context.Context, evt *Event) HandlerResult
}
