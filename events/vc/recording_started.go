// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"

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

func processVCRecordingStarted(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	body, ok := decodeEventBody[recordingBeanEventBody](raw)
	if !ok {
		return nil, processing.DropMalformed(raw.EventType)
	}
	if body.Source != recordingBeanSource {
		return nil, nil
	}
	out := &VCRecordingStartedOutput{
		Type:      raw.EventType,
		EventID:   raw.EventID,
		EventTime: millisToLocalRFC3339(raw.SourceTime),
		UniqueKey: body.UniqueKey,
		Source:    body.Source,
	}
	return json.Marshal(out)
}
