// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/larksuite/cli/internal/event"
)

// VCRecordingEndedOutput is the flattened shape for vc.recording.recording_ended_v1.
type VCRecordingEndedOutput struct {
	Type      string `json:"type"                 desc:"Event type; always vc.recording.recording_ended_v1"`
	EventID   string `json:"event_id,omitempty"   desc:"Globally unique event ID; safe for deduplication"`
	EventTime string `json:"event_time,omitempty" desc:"Time when the recording ended and uploaded successfully, in RFC3339 / ISO 8601 with the current system timezone"`
	UniqueKey string `json:"unique_key,omitempty" desc:"Unique key generated for one recording_bean recording session"`
	Source    string `json:"source,omitempty"     desc:"Recording source; always recording_bean"`
}

type recordingEndedEnvelope struct {
	Header struct {
		EventID    string `json:"event_id"`
		EventType  string `json:"event_type"`
		CreateTime string `json:"create_time"`
	} `json:"header"`
	Event recordingEndedEvent `json:"event"`
}

type recordingEndedEvent struct {
	UniqueKey string `json:"unique_key"`
	Source    string `json:"source"`
}

func processVCRecordingEnded(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	envelope, ok := parseRecordingEndedEnvelope(raw)
	if !ok {
		return raw.Payload, nil
	}
	if !isRecordingEndedBeanEvent(envelope) {
		return nil, nil
	}
	out := &VCRecordingEndedOutput{
		Type:      recordingEndedEventType(envelope, raw),
		EventID:   envelope.Header.EventID,
		EventTime: recordingEndedEventTime(envelope.Header.CreateTime),
		UniqueKey: envelope.Event.UniqueKey,
		Source:    envelope.Event.Source,
	}
	return json.Marshal(out)
}

func parseRecordingEndedEnvelope(raw *event.RawEvent) (*recordingEndedEnvelope, bool) {
	var envelope recordingEndedEnvelope
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return nil, false
	}
	return &envelope, true
}

func isRecordingEndedBeanEvent(envelope *recordingEndedEnvelope) bool {
	return envelope != nil && envelope.Event.Source == "recording_bean"
}

func recordingEndedEventType(envelope *recordingEndedEnvelope, raw *event.RawEvent) string {
	if envelope != nil && envelope.Header.EventType != "" {
		return envelope.Header.EventType
	}
	return raw.EventType
}

func recordingEndedEventTime(raw string) string {
	if raw == "" {
		return ""
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return ""
	}
	return time.UnixMilli(millis).Local().Format(time.RFC3339)
}
