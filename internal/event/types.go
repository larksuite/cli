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

	Params []ParamDef `json:"params,omitempty"`

	Schema SchemaDef `json:"schema"`

	// Process required when Schema.Custom is Processed output; must be nil when Native is used.
	Process func(ctx context.Context, rt APIClient, raw *RawEvent, params map[string]string) (json.RawMessage, error) `json:"-"`

	PreConsume func(ctx context.Context, rt APIClient, params map[string]string) (cleanup func(), err error) `json:"-"`

	Scopes []string `json:"scopes,omitempty"`

	// AuthTypes: whitelist of identities the EventKey accepts. Empty = no identity required.
	AuthTypes []string `json:"auth_types,omitempty"`

	RequiredConsoleEvents []string `json:"required_console_events,omitempty"`

	BufferSize int `json:"buffer_size,omitempty"`
	Workers    int `json:"workers,omitempty"`
}
