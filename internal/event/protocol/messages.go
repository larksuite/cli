// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package protocol

import "encoding/json"

// Message type constants.
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

// Source state constants (SourceStatus.State).
const (
	SourceStateConnecting   = "connecting"
	SourceStateConnected    = "connected"
	SourceStateDisconnected = "disconnected"
	SourceStateReconnecting = "reconnecting"
)

// SourceStatus is sent Bus → consume to surface WebSocket / source-level
// lifecycle events to the user. Best-effort: the hub drops the message
// when a consumer's send channel is full.
type SourceStatus struct {
	Type   string `json:"type"`
	Source string `json:"source"` // e.g. "feishu-websocket"
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"` // free-form, e.g. "attempt 1", "connection reset by peer"
}

// Hello is sent by consume → Bus on connect.
type Hello struct {
	Type       string   `json:"type"`
	PID        int      `json:"pid"`
	EventKey   string   `json:"event_key"`
	EventTypes []string `json:"event_types"`
	Version    string   `json:"version"`
}

// HelloAck is sent by Bus → consume after Hello.
type HelloAck struct {
	Type        string `json:"type"`
	BusVersion  string `json:"bus_version"`
	FirstForKey bool   `json:"first_for_key"`
}

// Event is sent by Bus → consume for each routed event.
//
// EventID and SourceTime replicate the corresponding fields on the upstream
// RawEvent so the consumer can attribute each message without having to
// peek into the Feishu-specific envelope inside Payload. Both are omitted
// from the wire when empty (omitempty) so old consumers reading new bytes
// simply ignore them (Go's default JSON unmarshal behaviour).
//
// Seq is a per-connection monotonically increasing counter assigned by
// Hub.Publish. A gap in the seq stream at the consumer side means the bus
// dropped events via the drop-oldest backpressure path on sendCh; the
// consumer logs a WARN with the gap size so silent data loss is detectable.
type Event struct {
	Type       string          `json:"type"`
	EventType  string          `json:"event_type"`
	EventID    string          `json:"event_id,omitempty"`
	SourceTime string          `json:"source_time,omitempty"` // ms-precision unix timestamp, stringified
	Seq        uint64          `json:"seq,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

// Bye is sent by consume → Bus before graceful disconnect.
type Bye struct {
	Type string `json:"type"`
}

// PreShutdownCheck is sent by consume → Bus to atomically reserve a
// cleanup lock for the consumer's EventKey. Bus replies with
// PreShutdownAck{LastForKey}. A true ack means the caller has exclusive
// cleanup rights until it disconnects (or the bus releases on its
// behalf); false means another subscriber beat us and cleanup should not
// run. See checkLastForKey in internal/event/consume/shutdown.go for the
// caller semantics and internal/event/bus/hub.go AcquireCleanupLock for
// the bus-side atomicity guarantee.
type PreShutdownCheck struct {
	Type     string `json:"type"`
	EventKey string `json:"event_key"`
}

// PreShutdownAck is sent by Bus → consume in response to PreShutdownCheck.
// LastForKey=true means "you acquired the cleanup reservation". Under the
// current protocol this is equivalent to "you were the last subscriber at
// the time of the ack AND no concurrent cleanup was in progress".
type PreShutdownAck struct {
	Type       string `json:"type"`
	LastForKey bool   `json:"last_for_key"`
}

// StatusQuery is sent by "event stop/status" → Bus.
type StatusQuery struct {
	Type string `json:"type"`
}

// ConsumerInfo describes a single connected consumer.
type ConsumerInfo struct {
	PID      int    `json:"pid"`
	EventKey string `json:"event_key"`
	Received int64  `json:"received"` // events fanned out by Hub to this consumer
	Dropped  int64  `json:"dropped"`  // events evicted by drop-oldest backpressure on this conn
}

// StatusResponse is sent by Bus → "event stop/status".
type StatusResponse struct {
	Type        string         `json:"type"`
	PID         int            `json:"pid"`
	UptimeSec   int            `json:"uptime_sec"`
	ActiveConns int            `json:"active_conns"`
	Consumers   []ConsumerInfo `json:"consumers"`
}

// Shutdown is sent by "event stop" → Bus to request graceful shutdown.
type Shutdown struct {
	Type string `json:"type"`
}

// ----- Constructors -----
//
// These helpers set Type automatically so callers can't silently produce
// messages missing the discriminator (the wire format requires `type` —
// Decode rejects messages without it). Prefer these over struct literals
// in production code; tests may still use literals when they deliberately
// exercise malformed frames.

// NewHello creates a Hello message with Type pre-filled.
func NewHello(pid int, eventKey string, eventTypes []string, version string) *Hello {
	return &Hello{
		Type:       MsgTypeHello,
		PID:        pid,
		EventKey:   eventKey,
		EventTypes: eventTypes,
		Version:    version,
	}
}

// NewHelloAck creates a HelloAck with Type pre-filled.
func NewHelloAck(busVersion string, firstForKey bool) *HelloAck {
	return &HelloAck{
		Type:        MsgTypeHelloAck,
		BusVersion:  busVersion,
		FirstForKey: firstForKey,
	}
}

// NewEvent creates an Event with Type pre-filled.
func NewEvent(eventType, eventID, sourceTime string, seq uint64, payload json.RawMessage) *Event {
	return &Event{
		Type:       MsgTypeEvent,
		EventType:  eventType,
		EventID:    eventID,
		SourceTime: sourceTime,
		Seq:        seq,
		Payload:    payload,
	}
}

// NewPreShutdownCheck creates a PreShutdownCheck with Type pre-filled.
func NewPreShutdownCheck(eventKey string) *PreShutdownCheck {
	return &PreShutdownCheck{Type: MsgTypePreShutdownCheck, EventKey: eventKey}
}

// NewPreShutdownAck creates a PreShutdownAck with Type pre-filled.
func NewPreShutdownAck(lastForKey bool) *PreShutdownAck {
	return &PreShutdownAck{Type: MsgTypePreShutdownAck, LastForKey: lastForKey}
}

// NewStatusQuery creates a StatusQuery message.
func NewStatusQuery() *StatusQuery {
	return &StatusQuery{Type: MsgTypeStatusQuery}
}

// NewStatusResponse creates a StatusResponse with Type pre-filled.
func NewStatusResponse(pid int, uptimeSec int, activeConns int, consumers []ConsumerInfo) *StatusResponse {
	return &StatusResponse{
		Type:        MsgTypeStatusResponse,
		PID:         pid,
		UptimeSec:   uptimeSec,
		ActiveConns: activeConns,
		Consumers:   consumers,
	}
}

// NewShutdown creates a Shutdown message.
func NewShutdown() *Shutdown { return &Shutdown{Type: MsgTypeShutdown} }

// NewSourceStatus creates a SourceStatus with Type pre-filled.
func NewSourceStatus(source, state, detail string) *SourceStatus {
	return &SourceStatus{
		Type:   MsgTypeSourceStatus,
		Source: source,
		State:  state,
		Detail: detail,
	}
}
