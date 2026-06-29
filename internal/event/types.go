// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package event owns the EventKey registry, RawEvent, APIClient, and dedup filter.
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

// RawEvent: SourceTime is upstream create_time; Timestamp is local source observation time.
type RawEvent struct {
	EventID    string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	SourceTime string          `json:"source_time,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	Timestamp  time.Time       `json:"timestamp"`
}

// APIClient: identity is opaque so business code can't bypass pre-flight checks.
type APIClient interface {
	CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error)
}

type ParamType string

const (
	ParamString ParamType = "string"
	ParamEnum   ParamType = "enum"
	ParamMulti  ParamType = "multi"
	ParamBool   ParamType = "bool"
	ParamInt    ParamType = "int"
)

// SubscriptionType marks whether an EventKey is delivered via Lark event
// subscription or interactive callback subscription. It is a sibling of
// EventType (which holds the concrete Lark event_type string).
type SubscriptionType string

const (
	// SubTypeEvent: checked against the published app_versions event_infos.
	SubTypeEvent SubscriptionType = "event"
	// SubTypeCallback: checked against application/get subscribed_callbacks.
	SubTypeCallback SubscriptionType = "callback"
)

// ParamValue.Desc is mandatory so AI consumers can decide which value to pick.
type ParamValue struct {
	Value string `json:"value"`
	Desc  string `json:"desc"`
}

type ParamDef struct {
	Name        string       `json:"name"`
	Type        ParamType    `json:"type"`
	Required    bool         `json:"required"`
	Default     string       `json:"default,omitempty"`
	Description string       `json:"description"`
	Values      []ParamValue `json:"values,omitempty"`

	// SubscriptionKey marks this param as part of the subscription identity.
	// Two consumers of the same EventKey but different values for any
	// SubscriptionKey-marked param are treated as DISTINCT subscriptions:
	// PreConsume runs once per (EventKey, SubscriptionID), cleanup runs once per
	// (EventKey, SubscriptionID).
	//
	// CONTRACT: only mark a param SubscriptionKey if the EventKey's server-side
	// subscribe/unsubscribe API is itself scoped to that resource. Lark keys the
	// subscription record by (app, user, event_type) and overwrites it rather
	// than reference-counting, so for a non-per-resource API the cleanup of one
	// resource's last consumer unsubscribes the shared record and silently cuts
	// off every other resource sharing that event_type.
	//
	// Default false = the param is a filter / formatting / metadata param
	// and does not affect subscription identity.
	SubscriptionKey bool `json:"subscription_key,omitempty"`
}

type ProcessFunc = func(ctx context.Context, rt APIClient, raw *RawEvent, params map[string]string) (json.RawMessage, error)

// SchemaDef: exactly one of Native or Custom must be set.
// Native auto-wraps the SDK type in the V2 envelope; Custom passes through verbatim.
type SchemaDef struct {
	Native         *SchemaSpec                  `json:"native,omitempty"`
	Custom         *SchemaSpec                  `json:"custom,omitempty"`
	FieldOverrides map[string]schemas.FieldMeta `json:"field_overrides,omitempty"`
}

// SchemaSpec: exactly one of Type or Raw.
type SchemaSpec struct {
	Type reflect.Type    `json:"-"`
	Raw  json.RawMessage `json:"raw,omitempty"`
}

type KeyDefinition struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	EventType   string `json:"event_type"`

	// SubscriptionType selects which console "底账" the precheck reads.
	// Empty is normalized to SubTypeEvent at RegisterKey.
	SubscriptionType SubscriptionType `json:"subscription_type,omitempty"`

	Params []ParamDef `json:"params,omitempty"`

	Schema SchemaDef `json:"schema"`

	// NormalizeParams canonicalizes param values BEFORE fingerprint compute,
	// PreConsume, Match, and Process. Mutates the params map in place.
	// May call OAPI; runs once per consumer at startup.
	//
	// Use cases: resolve aliases ("me" -> real email, a name -> an ID),
	// trim whitespace. On error, consume fails (no retry); caller gets the
	// wrapped error.
	//
	// Default nil = no normalization, params pass through unchanged.
	NormalizeParams func(ctx context.Context, rt APIClient, params map[string]string) error `json:"-"`

	// Process required when Schema.Custom is Processed output; must be nil when Native is used.
	//
	// Convention: returning (nil, nil) signals "drop this event" — the
	// consumer loop will skip writing it to sink and not advance the
	// emitted counter. Useful for async filtering (e.g. fetch metadata,
	// drop if folder doesn't match). For sync filters that don't need
	// OAPI, use Match instead.
	Process func(ctx context.Context, rt APIClient, raw *RawEvent, params map[string]string) (json.RawMessage, error) `json:"-"`

	// Match is a synchronous payload filter run on every received event
	// BEFORE Process. Return false to drop the event without further work.
	//
	// Signature deliberately omits ctx/rt to physically enforce "no OAPI
	// calls in Match". For filters that need a metadata fetch first, use
	// Process and return nil to drop.
	//
	// Default nil = accept all events.
	Match func(raw *RawEvent, params map[string]string) bool `json:"-"`

	// PreConsume runs once per (EventKey, SubscriptionID) when this consumer
	// is first for that scope. Returns a cleanup function that the framework
	// invokes when this consumer is the last for its scope.
	//
	// The cleanup's error return is honored: on nil the framework prints
	// "[event] cleanup done."; on non-nil it prints a WARN with an
	// idempotency note.
	PreConsume func(ctx context.Context, rt APIClient, params map[string]string) (cleanup func() error, err error) `json:"-"`

	Scopes []string `json:"scopes,omitempty"`

	// AuthTypes: whitelist of identities the EventKey accepts. Empty = no identity required.
	AuthTypes []string `json:"auth_types,omitempty"`

	RequiredConsoleEvents []string `json:"required_console_events,omitempty"`

	BufferSize int `json:"buffer_size,omitempty"`
	Workers    int `json:"workers,omitempty"`

	// SingleConsumer rejects a second consumer for the same SubscriptionID at
	// the bus handshake. Default false = unlimited consumers (fan-out).
	SingleConsumer bool `json:"single_consumer,omitempty"`
}
