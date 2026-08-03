// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package protocol

import (
	"encoding/json"
	"time"

	"github.com/larksuite/cli/internal/event/model"
)

const (
	MsgTypeHello            = "hello"
	MsgTypeHelloAck         = "hello_ack"
	MsgTypeEvent            = "event"
	MsgTypeBye              = "bye"
	MsgTypePreShutdownCheck = "pre_shutdown_check"
	MsgTypePreShutdownAck   = "pre_shutdown_ack"
	MsgTypeStatusQuery      = "status_query"
	MsgTypeStatusResponse   = "status_response"
	MsgTypeShutdown         = "shutdown"
	MsgTypeSourceStatus     = "source_status"
)

const (
	SourceStateConnecting   = "connecting"
	SourceStateConnected    = "connected"
	SourceStateDisconnected = "disconnected"
	SourceStateReconnecting = "reconnecting"
)

// SourceStatus is best-effort: hub drops it when consumer's send channel is full.
type SourceStatus struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type Hello struct {
	Type           string   `json:"type"`
	PID            int      `json:"pid"`
	EventKey       string   `json:"event_key"`
	EventTypes     []string `json:"event_types"`
	Version        string   `json:"version"`
	SubscriptionID string   `json:"subscription_id,omitempty"` // empty = fallback to EventKey on bus side
}

// CapabilityCanonicalMetadataV1 declares that every event frame this bus
// publishes carries the full canonical metadata set (event id, source time,
// tenant identity, observation time). Consumers that depend on those facts
// verify the capability on the delivery connection's ack and refuse to attach
// to a bus that cannot provide them.
const CapabilityCanonicalMetadataV1 = "canonical_metadata_v1"

type HelloAck struct {
	Type         string `json:"type"`
	BusVersion   string `json:"bus_version"`
	FirstForKey  bool   `json:"first_for_key"`
	Rejected     bool   `json:"rejected,omitempty"`
	RejectReason string `json:"reject_reason,omitempty"`
	// Capabilities is additive: an older bus simply never sends it, which is
	// exactly the signal consumers use to reject the attach.
	Capabilities []string `json:"capabilities,omitempty"`
}

// Event: Seq is per-conn monotonic; gaps signal bus drop-oldest backpressure loss.
// The frame carries every canonical fact the ingress parsed — consumers restore
// them verbatim instead of re-deriving anything from the payload. All fields
// beyond the original set are additive so older peers ignore them.
type Event struct {
	Type       string `json:"type"`
	EventType  string `json:"event_type"`
	EventID    string `json:"event_id,omitempty"`
	SourceTime string `json:"source_time,omitempty"` // upstream create_time verbatim; empty when the upstream omitted it
	AppID      string `json:"app_id,omitempty"`
	TenantKey  string `json:"tenant_key,omitempty"`
	// ObservedAt is the ingress observation clock in RFC3339Nano — a fixed
	// string contract on the wire, not whatever time.Time happens to marshal to.
	ObservedAt string          `json:"observed_at,omitempty"`
	Seq        uint64          `json:"seq,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

type Bye struct {
	Type string `json:"type"`
}

// PreShutdownCheck atomically reserves the cleanup lock for (EventKey, SubscriptionID).
type PreShutdownCheck struct {
	Type           string `json:"type"`
	EventKey       string `json:"event_key"`
	SubscriptionID string `json:"subscription_id,omitempty"` // empty = fallback to EventKey
}

type PreShutdownAck struct {
	Type       string `json:"type"`
	LastForKey bool   `json:"last_for_key"`
}

type StatusQuery struct {
	Type string `json:"type"`
}

type ConsumerInfo struct {
	PID            int    `json:"pid"`
	EventKey       string `json:"event_key"`
	SubscriptionID string `json:"subscription_id,omitempty"`
	Received       int64  `json:"received"`
	Dropped        int64  `json:"dropped"`
}

type StatusResponse struct {
	Type        string         `json:"type"`
	PID         int            `json:"pid"`
	UptimeSec   int            `json:"uptime_sec"`
	ActiveConns int            `json:"active_conns"`
	Consumers   []ConsumerInfo `json:"consumers"`
}

type Shutdown struct {
	Type string `json:"type"`
}

func NewHello(pid int, eventKey string, eventTypes []string, version string, subscriptionID string) *Hello {
	return &Hello{
		Type:           MsgTypeHello,
		PID:            pid,
		EventKey:       eventKey,
		EventTypes:     eventTypes,
		Version:        version,
		SubscriptionID: subscriptionID,
	}
}

func NewHelloAck(busVersion string, firstForKey bool, capabilities ...string) *HelloAck {
	return &HelloAck{
		Type:         MsgTypeHelloAck,
		BusVersion:   busVersion,
		FirstForKey:  firstForKey,
		Capabilities: capabilities,
	}
}

// NewHelloAckRejected builds a hello_ack that tells the consumer the bus refused
// registration (e.g. a SingleConsumer EventKey already has a running consumer).
func NewHelloAckRejected(busVersion, reason string) *HelloAck {
	return &HelloAck{
		Type:         MsgTypeHelloAck,
		BusVersion:   busVersion,
		Rejected:     true,
		RejectReason: reason,
	}
}

// NewEvent projects the canonical event onto the wire frame verbatim. It is
// the only Event constructor on purpose: every fact travels or is visibly
// absent — nothing is defaulted, substituted, or dropped here.
func NewEvent(ev *model.Event, seq uint64) *Event {
	observedAt := ""
	if !ev.Timestamp.IsZero() {
		// UTC-normalized so the wire never carries the emitting host's local
		// offset; consumers parse RFC3339Nano either way, but the frame bytes
		// should not depend on where the bus happens to run.
		observedAt = ev.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	return &Event{
		Type:       MsgTypeEvent,
		EventType:  ev.EventType,
		EventID:    ev.EventID,
		SourceTime: ev.SourceTime,
		AppID:      ev.AppID,
		TenantKey:  ev.TenantKey,
		ObservedAt: observedAt,
		Seq:        seq,
		Payload:    ev.Payload,
	}
}

func NewPreShutdownCheck(eventKey, subscriptionID string) *PreShutdownCheck {
	return &PreShutdownCheck{Type: MsgTypePreShutdownCheck, EventKey: eventKey, SubscriptionID: subscriptionID}
}

func NewPreShutdownAck(lastForKey bool) *PreShutdownAck {
	return &PreShutdownAck{Type: MsgTypePreShutdownAck, LastForKey: lastForKey}
}

func NewStatusQuery() *StatusQuery {
	return &StatusQuery{Type: MsgTypeStatusQuery}
}

func NewStatusResponse(pid int, uptimeSec int, activeConns int, consumers []ConsumerInfo) *StatusResponse {
	return &StatusResponse{
		Type:        MsgTypeStatusResponse,
		PID:         pid,
		UptimeSec:   uptimeSec,
		ActiveConns: activeConns,
		Consumers:   consumers,
	}
}

func NewShutdown() *Shutdown { return &Shutdown{Type: MsgTypeShutdown} }

func NewSourceStatus(source, state, detail string) *SourceStatus {
	return &SourceStatus{
		Type:   MsgTypeSourceStatus,
		Source: source,
		State:  state,
		Detail: detail,
	}
}
