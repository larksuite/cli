// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "fmt"

type builtinEventRuntime interface {
	EventHandler
	EventProcessor
}

// ProcessorRegistry manages event_type → EventProcessor mappings.
type ProcessorRegistry struct {
	processors map[string]EventProcessor
	fallback   EventProcessor
}

// HandlerRegistry manages event and domain scoped EventHandler registrations.
type HandlerRegistry struct {
	eventHandlers  map[string][]EventHandler
	domainHandlers map[string][]EventHandler
	fallback       EventHandler
	ids            map[string]struct{}
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
		return fmt.Errorf("duplicate event processor for: %s", et)
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
	r := NewProcessorRegistry(NewGenericFallbackHandler())
	for _, p := range builtinEventRuntimes() {
		_ = r.Register(p)
	}
	return r
}

// NewHandlerRegistry creates an empty handler registry.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		eventHandlers:  make(map[string][]EventHandler),
		domainHandlers: make(map[string][]EventHandler),
		ids:            make(map[string]struct{}),
	}
}

// NewBuiltinHandlerRegistry creates a handler registry with the built-in runtime handlers.
func NewBuiltinHandlerRegistry() *HandlerRegistry {
	r := NewHandlerRegistry()
	for _, h := range builtinEventRuntimes() {
		_ = r.RegisterEventHandler(h)
	}
	_ = r.SetFallbackHandler(NewGenericFallbackHandler())
	return r
}

func builtinEventRuntimes() []builtinEventRuntime {
	return []builtinEventRuntime{
		NewIMMessageReceiveHandler(),
		NewIMMessageReadHandler(),
		NewIMReactionCreatedHandler(),
		NewIMReactionDeletedHandler(),
		NewIMChatUpdatedHandler(),
		NewIMChatDisbandedHandler(),
		NewIMChatMemberBotAddedHandler(),
		NewIMChatMemberBotDeletedHandler(),
		NewIMChatMemberUserAddedHandler(),
		NewIMChatMemberUserWithdrawnHandler(),
		NewIMChatMemberUserDeletedHandler(),
	}
}

func builtinEventTypes() []string {
	runtimes := builtinEventRuntimes()
	types := make([]string, 0, len(runtimes))
	for _, runtime := range runtimes {
		if eventType := runtime.EventType(); eventType != "" {
			types = append(types, eventType)
		}
	}
	return types
}

// RegisterEventHandler registers a handler for an exact event type.
func (r *HandlerRegistry) RegisterEventHandler(h EventHandler) error {
	if err := r.validateHandler(h, h.EventType(), "event type"); err != nil {
		return err
	}
	r.eventHandlers[h.EventType()] = append(r.eventHandlers[h.EventType()], h)
	return nil
}

// RegisterDomainHandler registers a handler for an exact domain.
func (r *HandlerRegistry) RegisterDomainHandler(h EventHandler) error {
	if err := r.validateHandler(h, h.Domain(), "domain"); err != nil {
		return err
	}
	r.domainHandlers[h.Domain()] = append(r.domainHandlers[h.Domain()], h)
	return nil
}

// SetFallbackHandler sets the fallback handler used when no handlers match.
func (r *HandlerRegistry) SetFallbackHandler(h EventHandler) error {
	if err := r.validateHandler(h, h.ID(), "fallback handler"); err != nil {
		return err
	}
	r.fallback = h
	return nil
}

// EventHandlers returns handlers registered for the exact event type.
func (r *HandlerRegistry) EventHandlers(eventType string) []EventHandler {
	if r == nil {
		return nil
	}
	return r.eventHandlers[eventType]
}

// DomainHandlers returns handlers registered for the exact domain.
func (r *HandlerRegistry) DomainHandlers(domain string) []EventHandler {
	if r == nil {
		return nil
	}
	return r.domainHandlers[domain]
}

// FallbackHandler returns the configured fallback handler.
func (r *HandlerRegistry) FallbackHandler() EventHandler {
	if r == nil {
		return nil
	}
	return r.fallback
}

func (r *HandlerRegistry) validateHandler(h EventHandler, key string, scope string) error {
	if r == nil {
		return fmt.Errorf("nil handler registry")
	}
	if h == nil {
		return fmt.Errorf("nil handler")
	}
	if h.ID() == "" {
		return fmt.Errorf("handler ID is required")
	}
	if key == "" {
		return fmt.Errorf("handler %q %s is required", h.ID(), scope)
	}
	if _, exists := r.ids[h.ID()]; exists {
		return fmt.Errorf("duplicate handler ID: %s", h.ID())
	}
	r.ids[h.ID()] = struct{}{}
	return nil
}
