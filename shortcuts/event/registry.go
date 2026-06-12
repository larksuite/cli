// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "github.com/larksuite/cli/errs"

// ProcessorRegistry manages event_type → EventProcessor mappings.
type ProcessorRegistry struct {
	processors map[string]EventProcessor
	fallback   EventProcessor
}

// NewProcessorRegistry creates a registry with a fallback for unregistered event types.
func NewProcessorRegistry(fallback EventProcessor) *ProcessorRegistry {
	return &ProcessorRegistry{
		processors: make(map[string]EventProcessor),
		fallback:   fallback,
	}
}

// Register adds a processor. Returns an error on duplicate event type registration.
func (r *ProcessorRegistry) Register(p EventProcessor) error {
	et := p.EventType()
	if _, exists := r.processors[et]; exists {
		return errs.NewInternalError(errs.SubtypeUnknown, "duplicate event processor for: %s", et)
	}
	r.processors[et] = p
	return nil
}

// Lookup finds a processor by event type. Returns fallback if not registered. Never returns nil.
func (r *ProcessorRegistry) Lookup(eventType string) EventProcessor {
	if p, ok := r.processors[eventType]; ok {
		return p
	}
	return r.fallback
}

// DefaultRegistry builds the standard processor registry.
// To add a new processor, just add r.Register(...) here.
func DefaultRegistry() *ProcessorRegistry {
	r := NewProcessorRegistry(&GenericProcessor{})
	// im.message
	_ = r.Register(&ImMessageProcessor{})
	_ = r.Register(&ImMessageReadProcessor{})
	_ = r.Register(NewImReactionCreatedProcessor())
	_ = r.Register(NewImReactionDeletedProcessor())
	// im.chat.member
	_ = r.Register(NewImChatBotAddedProcessor())
	_ = r.Register(NewImChatBotDeletedProcessor())
	_ = r.Register(NewImChatMemberUserAddedProcessor())
	_ = r.Register(NewImChatMemberUserWithdrawnProcessor())
	_ = r.Register(NewImChatMemberUserDeletedProcessor())
	// im.chat
	_ = r.Register(&ImChatUpdatedProcessor{})
	_ = r.Register(&ImChatDisbandedProcessor{})
	return r
}
