// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package event is the hub of the Lark CLI event subsystem: it owns the
// registry of EventKeys, the RawEvent type that sources emit, the
// APIClient interface that EventKey PreConsume hooks call through, and
// the dedup filter used by the bus to drop SDK replays.
package event

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/larksuite/cli/internal/event/schemas"
)

const (
	DefaultBufferSize = 100
	MaxBufferSize     = 1000
)

// RawEvent is the unit of data flowing through the Bus.
//
// Timestamp records "when we (the source) saw it locally", useful for
// bus-internal observability. SourceTime is the upstream publisher's
// create_time (ms unix timestamp, stringified) propagated verbatim to the
// consumer as protocol.Event.SourceTime — preferred over Timestamp when
// set because it preserves upstream semantics even if the source reorders
// or batches delivery locally.
type RawEvent struct {
	EventID    string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	SourceTime string          `json:"source_time,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	Timestamp  time.Time       `json:"timestamp"`
}

// APIClient provides API access to Process and PreConsume functions.
// Intentionally narrow — avoids importing shortcuts/common.Runtime so the
// event package stays self-contained. The concrete implementation (in
// cmd/event/consume.go) wraps the project's unified client.APIClient so
// calls go through the same SDK stack as every other command (UA,
// retries, tracing, permission diagnostics).
type APIClient interface {
	// CallAPI invokes a Lark Open API and returns the raw JSON response
	// body. The identity is whatever the consume command resolved from
	// the EventKey's AuthTypes (--as flag, config default, etc.) —
	// implementations do not expose it so business code can't pin a
	// different identity and skip the pre-flight checks.
	CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error)
}

// ParamType classifies param value semantics. Enum/Multi require Values.
type ParamType string

const (
	ParamString ParamType = "string" // free text
	ParamEnum   ParamType = "enum"   // single-pick from Values
	ParamMulti  ParamType = "multi"  // multi-pick from Values (comma separated)
	ParamBool   ParamType = "bool"
	ParamInt    ParamType = "int"
)

// ParamValue is one allowed value of an Enum/Multi param. Desc is
// mandatory so AI consumers can decide which value to pick.
type ParamValue struct {
	Value string `json:"value"`
	Desc  string `json:"desc"`
}

// ParamDef describes a --param key=value parameter for a business EventKey.
type ParamDef struct {
	Name        string       `json:"name"`
	Type        ParamType    `json:"type"`
	Required    bool         `json:"required"`
	Default     string       `json:"default,omitempty"`
	Description string       `json:"description"`
	Values      []ParamValue `json:"values,omitempty"`
}

// ProcessFunc is the type of the business logic function for an EventKey.
type ProcessFunc = func(ctx context.Context, rt APIClient, raw *RawEvent, params map[string]string) (json.RawMessage, error)

// SchemaDef describes how an EventKey's delivered payload is typed.
// Exactly one of Native or Custom must be non-nil.
type SchemaDef struct {
	// Native: the SDK struct describing the `event` body. Framework
	// auto-wraps it in the V2 envelope (schema/header/event). Use for
	// the "zero-effort" path — business just points at the SDK type.
	Native *SchemaSpec `json:"native,omitempty"`

	// Custom: the complete schema consumers see. Framework does not
	// modify it. Use for Processed keys (Process produces this shape)
	// or Native keys where business wants to hand-author the envelope.
	Custom *SchemaSpec `json:"custom,omitempty"`

	// FieldOverrides is a pointer-keyed semantic overlay applied after
	// reflection (and envelope wrap, for Native). Paths that do not
	// resolve are reported by CI lint. See internal/event/schemas.
	FieldOverrides map[string]schemas.FieldMeta `json:"field_overrides,omitempty"`
}

// SchemaSpec points at a schema source — exactly one of Type or Raw.
type SchemaSpec struct {
	Type reflect.Type    `json:"-"`
	Raw  json.RawMessage `json:"raw,omitempty"`
}

// KeyDefinition is the registration unit for event keys.
type KeyDefinition struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	EventType   string `json:"event_type"`

	Params []ParamDef `json:"params,omitempty"`

	// Schema describes the payload shape consumers receive.
	Schema SchemaDef `json:"schema"`

	// Process is the business logic function. Required when Schema.Custom
	// carries Processed output; must be nil when Schema.Native is used.
	Process func(ctx context.Context, rt APIClient, raw *RawEvent, params map[string]string) (json.RawMessage, error) `json:"-"`

	// PreConsume runs before consumption starts. Returns a cleanup function.
	PreConsume func(ctx context.Context, rt APIClient, params map[string]string) (cleanup func(), err error) `json:"-"`

	// --- Auth & Permission metadata (AI-readable) ---

	// Scopes lists the OAuth scopes required to consume this EventKey.
	// Used for pre-flight scope checks and displayed in `event schema`.
	Scopes []string `json:"scopes,omitempty"`

	// AuthTypes lists identities this EventKey supports, matching the
	// semantics already used by shortcut definitions. The first element is
	// the default used when --as and DefaultAs don't specify; the full
	// slice is the allowed set. An empty slice means "no identity required"
	// (rare — only for keys that neither subscribe via OAPI nor invoke
	// APIs in Process/PreConsume).
	//
	// Examples:
	//   []string{"bot"}          — IM events (bot-scoped subscription)
	//   []string{"user"}         — mail (subscribe API is UAT-only)
	//   []string{"bot", "user"}  — both identities supported, bot default
	AuthTypes []string `json:"auth_types,omitempty"`

	// RequiredConsoleEvents lists event types that must be enabled in the
	// Feishu developer console for this EventKey to receive events.
	RequiredConsoleEvents []string `json:"required_console_events,omitempty"`

	BufferSize int `json:"buffer_size,omitempty"`
	Workers    int `json:"workers,omitempty"`
}
