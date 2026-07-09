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

// VCRecordingTranscriptItemOutput is one flattened transcript item for recording events.
type VCRecordingTranscriptItemOutput struct {
	SpeakerName string `json:"speaker_name,omitempty" desc:"Speaker display name"`
	Text        string `json:"text,omitempty"         desc:"Transcript text"`
	StartTime   string `json:"start_time,omitempty"   desc:"Transcript item start time in RFC3339 / ISO 8601 with the current system timezone"`
	EndTime     string `json:"end_time,omitempty"     desc:"Transcript item end time in RFC3339 / ISO 8601 with the current system timezone"`
	SentenceID  string `json:"sentence_id,omitempty"  desc:"Transcript sentence ID"`
}

// VCRecordingTranscriptGeneratedOutput is the flattened shape for vc.recording.recording_transcript_generated_v1.
type VCRecordingTranscriptGeneratedOutput struct {
	Type            string                            `json:"type"                       desc:"Event type; always vc.recording.recording_transcript_generated_v1"`
	EventID         string                            `json:"event_id,omitempty"         desc:"Globally unique event ID; safe for deduplication"`
	EventTime       string                            `json:"event_time,omitempty"       desc:"Time when this batch of transcript items was generated, in RFC3339 / ISO 8601 with the current system timezone"`
	UniqueKey       string                            `json:"unique_key,omitempty"       desc:"Unique key generated for one recording_bean recording session"`
	Source          string                            `json:"source,omitempty"           desc:"Recording source; always recording_bean"`
	TranscriptItems []VCRecordingTranscriptItemOutput `json:"transcript_items,omitempty" desc:"Generated transcript items"`
}

type recordingTranscriptGeneratedEnvelope struct {
	Header struct {
		EventID    string `json:"event_id"`
		EventType  string `json:"event_type"`
		CreateTime string `json:"create_time"`
	} `json:"header"`
	Event recordingTranscriptGeneratedEvent `json:"event"`
}

type recordingTranscriptGeneratedEvent struct {
	UniqueKey       string                               `json:"unique_key"`
	Source          string                               `json:"source"`
	TranscriptItems []recordingTranscriptGeneratedItemIn `json:"transcript_items"`
}

type recordingTranscriptGeneratedItemIn struct {
	Speaker     *recordingTranscriptGeneratedSpeakerIn `json:"speaker"`
	Text        string                                 `json:"text"`
	StartTimeMs recordingTranscriptGeneratedString     `json:"start_time_ms"`
	EndTimeMs   recordingTranscriptGeneratedString     `json:"end_time_ms"`
	SentenceID  string                                 `json:"sentence_id"`
}

type recordingTranscriptGeneratedSpeakerIn struct {
	UserName string `json:"user_name"`
}

type recordingTranscriptGeneratedString string

func processVCRecordingTranscriptGenerated(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	envelope, ok := parseRecordingTranscriptGeneratedEnvelope(raw)
	if !ok {
		return raw.Payload, nil
	}
	if !isRecordingTranscriptGeneratedBeanEvent(envelope) {
		return nil, nil
	}
	out := &VCRecordingTranscriptGeneratedOutput{
		Type:            recordingTranscriptGeneratedEventType(envelope, raw),
		EventID:         envelope.Header.EventID,
		EventTime:       recordingTranscriptGeneratedEventTime(envelope.Header.CreateTime),
		UniqueKey:       envelope.Event.UniqueKey,
		Source:          envelope.Event.Source,
		TranscriptItems: recordingTranscriptItems(envelope.Event.TranscriptItems),
	}
	return json.Marshal(out)
}

func parseRecordingTranscriptGeneratedEnvelope(raw *event.RawEvent) (*recordingTranscriptGeneratedEnvelope, bool) {
	var envelope recordingTranscriptGeneratedEnvelope
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return nil, false
	}
	return &envelope, true
}

func isRecordingTranscriptGeneratedBeanEvent(envelope *recordingTranscriptGeneratedEnvelope) bool {
	return envelope != nil && envelope.Event.Source == "recording_bean"
}

func recordingTranscriptGeneratedEventType(envelope *recordingTranscriptGeneratedEnvelope, raw *event.RawEvent) string {
	if envelope != nil && envelope.Header.EventType != "" {
		return envelope.Header.EventType
	}
	return raw.EventType
}

func recordingTranscriptGeneratedEventTime(raw string) string {
	return recordingTranscriptGeneratedMillisToLocalRFC3339(raw)
}

func recordingTranscriptGeneratedMillisToLocalRFC3339(raw string) string {
	if raw == "" {
		return ""
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return ""
	}
	return time.UnixMilli(millis).Local().Format(time.RFC3339)
}

func recordingTranscriptItems(items []recordingTranscriptGeneratedItemIn) []VCRecordingTranscriptItemOutput {
	if len(items) == 0 {
		return nil
	}
	out := make([]VCRecordingTranscriptItemOutput, 0, len(items))
	for _, item := range items {
		out = append(out, recordingTranscriptItem(item))
	}
	return out
}

func recordingTranscriptItem(item recordingTranscriptGeneratedItemIn) VCRecordingTranscriptItemOutput {
	return VCRecordingTranscriptItemOutput{
		SpeakerName: recordingSpeakerName(item.Speaker),
		Text:        item.Text,
		StartTime:   recordingTranscriptGeneratedMillisToLocalRFC3339(item.StartTimeMs.String()),
		EndTime:     recordingTranscriptGeneratedMillisToLocalRFC3339(item.EndTimeMs.String()),
		SentenceID:  item.SentenceID,
	}
}

func recordingSpeakerName(speaker *recordingTranscriptGeneratedSpeakerIn) string {
	if speaker == nil {
		return ""
	}
	return speaker.UserName
}

func (s *recordingTranscriptGeneratedString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = recordingTranscriptGeneratedString(str)
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(data, &num); err != nil {
		return err
	}
	*s = recordingTranscriptGeneratedString(num.String())
	return nil
}

func (s recordingTranscriptGeneratedString) String() string {
	return string(s)
}
