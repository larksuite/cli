// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package protocol

import "encoding/json"

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
	Type       string   `json:"type"`
	PID        int      `json:"pid"`
	EventKey   string   `json:"event_key"`
	EventTypes []string `json:"event_types"`
	Version    string   `json:"version"`
}

type HelloAck struct {
	Type        string `json:"type"`
	BusVersion  string `json:"bus_version"`
	FirstForKey bool   `json:"first_for_key"`
}

// Event: Seq is per-conn monotonic; gaps signal bus drop-oldest backpressure loss.
type Event struct {
	Type       string          `json:"type"`
	EventType  string          `json:"event_type"`
	EventID    string          `json:"event_id,omitempty"`
	SourceTime string          `json:"source_time,omitempty"` // ms-precision unix timestamp, stringified
	Seq        uint64          `json:"seq,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

type Bye struct {
	Type string `json:"type"`
}

// PreShutdownCheck atomically reserves the cleanup lock for EventKey.
type PreShutdownCheck struct {
	Type     string `json:"type"`
	EventKey string `json:"event_key"`
}

type PreShutdownAck struct {
	Type       string `json:"type"`
	LastForKey bool   `json:"last_for_key"`
}

type StatusQuery struct {
	Type string `json:"type"`
}

type ConsumerInfo struct {
	PID      int    `json:"pid"`
	EventKey string `json:"event_key"`
	Received int64  `json:"received"`
	Dropped  int64  `json:"dropped"`
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

func NewHello(pid int, eventKey string, eventTypes []string, version string) *Hello {
	return &Hello{
		Type:       MsgTypeHello,
		PID:        pid,
		EventKey:   eventKey,
		EventTypes: eventTypes,
		Version:    version,
	}
}

func NewHelloAck(busVersion string, firstForKey bool) *HelloAck {
	return &HelloAck{
		Type:        MsgTypeHelloAck,
		BusVersion:  busVersion,
		FirstForKey: firstForKey,
	}
}

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

func NewPreShutdownCheck(eventKey string) *PreShutdownCheck {
	return &PreShutdownCheck{Type: MsgTypePreShutdownCheck, EventKey: eventKey}
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
