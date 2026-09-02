// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package model holds the pure value types of the event kernel. It may import
// the standard library only; every other event package builds on top of it.
package model

import (
	"encoding/json"
	"time"
)

// Event is the single canonical representation of one upstream event.
// The ingress adapter parses the envelope header exactly once and fills these
// fields; everything downstream propagates and validates them but never
// re-derives them from the payload.
type Event struct {
	// EventID is the upstream unique id — the dedup and diagnostics anchor.
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	// SourceTime is the upstream create_time. When the upstream omits it, it
	// stays visibly empty — it is never backfilled from a local clock.
	SourceTime string `json:"source_time,omitempty"`
	// AppID and TenantKey identify the tenant the event was delivered for,
	// as parsed from the envelope header at ingress.
	AppID     string          `json:"app_id,omitempty"`
	TenantKey string          `json:"tenant_key,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	// Timestamp is the local observation clock at ingress. SourceTime and
	// Timestamp are two different facts and never substitute for each other.
	Timestamp time.Time `json:"timestamp"`
}
