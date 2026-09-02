// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import (
	"context"
	"encoding/json"
	"slices"
	"sort"

	"github.com/larksuite/cli/internal/event/model"
	"github.com/larksuite/cli/internal/event/processing"
)

// Descriptor holds the declaration's display facts: everything list/schema
// render, nothing that executes. Domain is always resolved here even when the
// declaration left it empty.
type Descriptor struct {
	Key                   string
	Domain                string
	DisplayName           string
	Description           string
	EventType             string
	SubscriptionType      SubscriptionType
	Params                []ParamDef
	Scopes                []string
	AuthTypes             []string
	RequiredConsoleEvents []string
}

// OutputMode states which side of the output contract a key lives on.
type OutputMode string

const (
	// OutputNative delivers the raw V2 envelope verbatim.
	OutputNative OutputMode = "native"
	// OutputProcessed delivers what the key's processor emits — and only that.
	OutputProcessed OutputMode = "processed"
)

// OutputContract is the compiled promise about a key's stdout: the fully
// resolved schema and the jq root consumers address fields from. Resolving at
// compile time means an unresolvable schema or a dangling field override is a
// startup failure, not a silently degraded rendering.
type OutputContract struct {
	Mode       OutputMode
	SchemaJSON json.RawMessage
	JQRootPath string
}

// Capability describes how a key's delivery is provisioned and bounded, in
// serializable form: which preparation strategy readies it, how deliveries
// are buffered, and whether a second consumer is rejected.
type Capability struct {
	Preparation    StrategyRef
	BufferSize     int
	Workers        int
	SingleConsumer bool
}

// RuntimeBinding carries the declaration's executable hooks. It has no JSON
// tags on purpose: behavior never travels through a rendering path.
type RuntimeBinding struct {
	NormalizeParams func(ctx context.Context, rt processing.APIClient, params map[string]string) error
	Match           func(raw *model.Event, params map[string]string) bool
	Process         ProcessFunc
	PreConsume      func(ctx context.Context, rt processing.APIClient, params map[string]string) (cleanup func() error, err error)
}

// Entry is one compiled key: four projections composed read-only. Definition
// reassembles the canonical compatibility view from them, which doubles as
// the proof that the projection lost nothing.
type Entry struct {
	descriptor Descriptor
	output     OutputContract
	capability Capability
	binding    RuntimeBinding
	// canonical is the Canonicalize'd declaration the entry was compiled
	// from; Definition returns deep copies of it.
	canonical KeyDefinition
}

func (e *Entry) Descriptor() Descriptor {
	d := e.descriptor
	d.Params = cloneParams(e.descriptor.Params)
	d.Scopes = slices.Clone(e.descriptor.Scopes)
	d.AuthTypes = slices.Clone(e.descriptor.AuthTypes)
	d.RequiredConsoleEvents = slices.Clone(e.descriptor.RequiredConsoleEvents)
	return d
}

func (e *Entry) Output() OutputContract {
	o := e.output
	o.SchemaJSON = slices.Clone(e.output.SchemaJSON)
	return o
}

func (e *Entry) Capability() Capability { return e.capability }

func (e *Entry) Binding() RuntimeBinding { return e.binding }

// Definition returns the canonical compatibility view of the declaration this
// entry was compiled from. Callers may mutate the returned value freely.
func (e *Entry) Definition() *KeyDefinition {
	def := deepCopyDefinition(e.canonical)
	return &def
}

func cloneParams(params []ParamDef) []ParamDef {
	out := slices.Clone(params)
	for i := range out {
		out[i].Values = slices.Clone(params[i].Values)
	}
	return out
}

// Snapshot is the compiled, immutable catalog. Accessors return values or
// fresh copies — never pointers into the snapshot's own state.
type Snapshot struct {
	entries map[string]*Entry
	keys    []string // pre-sorted
}

// Keys returns every compiled key in stable sorted order.
func (s *Snapshot) Keys() []string { return slices.Clone(s.keys) }

// Len reports how many keys were compiled.
func (s *Snapshot) Len() int { return len(s.keys) }

// Resolve returns the compiled entry for key, with ok=false for a key the
// catalog does not have. Every projection (descriptor, output, capability,
// binding) is read off the returned entry, so facts about one key can never
// be paired with another key's.
func (s *Snapshot) Resolve(key string) (*Entry, bool) {
	e, ok := s.entries[key]
	return e, ok
}

// Entries returns all compiled entries in key order.
func (s *Snapshot) Entries() []*Entry {
	out := make([]*Entry, 0, len(s.keys))
	for _, k := range s.keys {
		out = append(out, s.entries[k])
	}
	return out
}

// Definitions returns the canonical compatibility view of every entry in key
// order. Each element is a fresh deep copy, like Entry.Definition.
func (s *Snapshot) Definitions() []*KeyDefinition {
	out := make([]*KeyDefinition, 0, len(s.keys))
	for _, k := range s.keys {
		out = append(out, s.entries[k].Definition())
	}
	return out
}

// Domains returns the sorted, deduplicated domain set across all entries.
func (s *Snapshot) Domains() []string {
	seen := map[string]bool{}
	for _, e := range s.entries {
		seen[e.descriptor.Domain] = true
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// EventTypes returns the sorted, deduplicated set of upstream event types —
// what a bus subscribes to the platform with.
func (s *Snapshot) EventTypes() []string {
	seen := map[string]bool{}
	for _, e := range s.entries {
		seen[e.descriptor.EventType] = true
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
