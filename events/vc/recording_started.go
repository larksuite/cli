// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/processing"
)

// VCRecordingStartedOutput is the flattened shape for vc.recording.recording_started_v1.
type VCRecordingStartedOutput struct {
	Type      string `json:"type"                 desc:"Event type; always vc.recording.recording_started_v1"`
	EventID   string `json:"event_id,omitempty"   desc:"Globally unique event ID; safe for deduplication"`
	EventTime string `json:"event_time,omitempty" desc:"Recording start time in RFC3339 / ISO 8601 with the current system timezone"`
	UniqueKey string `json:"unique_key,omitempty" desc:"Unique key generated for one recording_bean recording session"`
	Source    string `json:"source,omitempty"     desc:"Recording source; always recording_bean"`
}

type recordingStartedEnvelope struct {
	Event recordingStartedEvent `json:"event"`
}

type recordingStartedEvent struct {
	UniqueKey string `json:"unique_key"`
	Source    string `json:"source"`
}

func processVCRecordingStarted(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	envelope, ok := parseRecordingStartedEnvelope(raw)
	if !ok {
		return nil, processing.DropMalformed(raw.EventType)
	}
	if !isRecordingStartedBeanEvent(envelope) {
		return nil, nil
	}
	out := &VCRecordingStartedOutput{
		Type:      raw.EventType,
		EventID:   raw.EventID,
		EventTime: recordingStartedEventTime(raw.SourceTime),
		UniqueKey: envelope.Event.UniqueKey,
		Source:    envelope.Event.Source,
	}
	return json.Marshal(out)
}

func parseRecordingStartedEnvelope(raw *event.RawEvent) (*recordingStartedEnvelope, bool) {
	var envelope recordingStartedEnvelope
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return nil, false
	}
	return &envelope, true
}

func isRecordingStartedBeanEvent(envelope *recordingStartedEnvelope) bool {
	return envelope != nil && envelope.Event.Source == "recording_bean"
}

func recordingStartedEventTime(raw string) string {
	if raw == "" {
		return ""
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return ""
	}
	return time.UnixMilli(millis).Local().Format(time.RFC3339)
}
