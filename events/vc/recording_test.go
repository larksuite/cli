// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

func TestVCKeys_RecordingEventsRegistered(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, tc := range []struct {
		eventType string
	}{
		{eventTypeRecordingStarted},
		{eventTypeRecordingTranscriptGenerated},
		{eventTypeRecordingEnded},
	} {
		t.Run(tc.eventType, func(t *testing.T) {
			def, ok := event.Lookup(tc.eventType)
			if !ok {
				t.Fatalf("%s should be registered via Keys()", tc.eventType)
			}
			if def.Schema.Custom == nil {
				t.Error("Processed key must set Schema.Custom")
			}
			if def.Schema.Native != nil {
				t.Error("Processed key must not set Schema.Native")
			}
			if def.Process == nil {
				t.Error("Process must not be nil for processed key")
			}
			if def.PreConsume == nil {
				t.Error("PreConsume must not be nil for processed key")
			}
			if len(def.Scopes) != 1 || def.Scopes[0] != "vc:recording:read" {
				t.Errorf("Scopes = %v", def.Scopes)
			}
			if len(def.AuthTypes) != 1 || def.AuthTypes[0] != "user" {
				t.Errorf("AuthTypes = %v", def.AuthTypes)
			}
			if len(def.RequiredConsoleEvents) != 1 || def.RequiredConsoleEvents[0] != tc.eventType {
				t.Errorf("RequiredConsoleEvents = %v", def.RequiredConsoleEvents)
			}
			if !strings.Contains(def.Description, "recording_bean") {
				t.Errorf("Description should document recording_bean source, got %q", def.Description)
			}
			if !strings.Contains(def.Description, "connected to Feishu software") {
				t.Errorf("Description should document Feishu software connection requirement, got %q", def.Description)
			}
			if strings.Contains(def.Description, "future") || strings.Contains(def.Description, "software_recording") {
				t.Errorf("Description should not mention future sources, got %q", def.Description)
			}
			if tc.eventType == eventTypeRecordingEnded && (strings.Contains(def.Description, "object_type") || strings.Contains(def.Description, "object_id")) {
				t.Errorf("ended Description should not document object metadata, got %q", def.Description)
			}
			wantSchemaType := reflect.TypeOf(VCRecordingStartedOutput{})
			switch tc.eventType {
			case eventTypeRecordingTranscriptGenerated:
				wantSchemaType = reflect.TypeOf(VCRecordingTranscriptGeneratedOutput{})
			case eventTypeRecordingEnded:
				wantSchemaType = reflect.TypeOf(VCRecordingEndedOutput{})
			}
			if def.Schema.Custom.Type != wantSchemaType {
				t.Errorf("Custom schema Type = %v, want %v", def.Schema.Custom.Type, wantSchemaType)
			}
		})
	}
}

func TestProcessVCRecordingStarted(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	out := runRecordingProcess[VCRecordingStartedOutput](t, eventTypeRecordingStarted, processVCRecordingStarted, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_rec_start_001",
			"event_type": "vc.recording.recording_started_v1",
			"create_time": "1761782400000"
		},
		"event": {
			"unique_key": "recording_001",
			"source": "recording_bean"
		}
	}`)

	if out.Type != eventTypeRecordingStarted {
		t.Errorf("Type = %q", out.Type)
	}
	if out.EventID != "ev_rec_start_001" || out.EventTime != recordingTestEventTime(1761782400000) {
		t.Errorf("EventID/EventTime = %q/%q", out.EventID, out.EventTime)
	}
	if out.UniqueKey != "recording_001" || out.Source != "recording_bean" {
		t.Errorf("UniqueKey/Source = %q/%q", out.UniqueKey, out.Source)
	}
}

func TestProcessVCRecordingTranscriptGenerated(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	got := runRecordingProcessRaw(t, eventTypeRecordingTranscriptGenerated, processVCRecordingTranscriptGenerated, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_rec_transcript_001",
			"event_type": "vc.recording.recording_transcript_generated_v1",
			"create_time": "1761782400100"
		},
		"event": {
			"unique_key": "recording_001",
			"source": "recording_bean",
			"transcript_items": [
				{
					"speaker": {
						"id": {
							"open_id": "ou_0f8bf7acdf2ae69553ecbdbfbbd10a53",
							"union_id": "on_bc03f16d781bff4178a5d11e48eb1867",
							"user_id": null
						},
						"user_type": 100,
						"user_role": 1,
						"user_name": "Alice"
					},
					"text": "hello world",
					"language": "en_us",
					"start_time_ms": "1761782399000",
					"end_time_ms": "1761782400000",
					"sentence_id": "987654321"
				},
				{
					"speaker": {
						"user_name": "Bob"
					},
					"text": "second sentence",
					"language": "en_us",
					"start_time_ms": "1761782401000",
					"end_time_ms": "1761782402000",
					"sentence_id": "987654322"
				}
			]
		}
	}`)
	if got == nil {
		t.Fatal("Process output is nil")
	}
	var out VCRecordingTranscriptGeneratedOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid JSON: %v\nraw=%s", err, string(got))
	}

	if out.Type != eventTypeRecordingTranscriptGenerated {
		t.Errorf("Type = %q", out.Type)
	}
	if out.UniqueKey != "recording_001" || out.Source != "recording_bean" {
		t.Errorf("UniqueKey/Source = %q/%q", out.UniqueKey, out.Source)
	}
	if out.EventTime != recordingTestEventTime(1761782400100) {
		t.Errorf("EventTime = %q", out.EventTime)
	}
	if len(out.TranscriptItems) != 2 {
		t.Fatalf("TranscriptItems len = %d, want 2", len(out.TranscriptItems))
	}
	item := out.TranscriptItems[0]
	if item.SpeakerName != "Alice" || item.Text != "hello world" {
		t.Errorf("Transcript speaker/text = %q/%q", item.SpeakerName, item.Text)
	}
	if item.StartTime != recordingTestEventTime(1761782399000) || item.EndTime != recordingTestEventTime(1761782400000) {
		t.Errorf("Transcript timing = %q/%q", item.StartTime, item.EndTime)
	}
	if item.SentenceID != "987654321" {
		t.Errorf("SentenceID = %q, want 987654321", item.SentenceID)
	}
	if out.TranscriptItems[1].SpeakerName != "Bob" || out.TranscriptItems[1].SentenceID != "987654322" {
		t.Errorf("second transcript item = %+v", out.TranscriptItems[1])
	}
	itemJSON, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal transcript item: %v", err)
	}
	var itemFields map[string]any
	if err := json.Unmarshal(itemJSON, &itemFields); err != nil {
		t.Fatalf("unmarshal transcript item JSON: %v", err)
	}
	wantItemFields := map[string]bool{
		"speaker_name": true,
		"text":         true,
		"start_time":   true,
		"end_time":     true,
		"sentence_id":  true,
	}
	for gotField := range itemFields {
		if !wantItemFields[gotField] {
			t.Errorf("Transcript item should not contain field %q, got %s", gotField, string(itemJSON))
		}
	}
	for wantField := range wantItemFields {
		if _, ok := itemFields[wantField]; !ok {
			t.Errorf("Transcript item missing field %q, got %s", wantField, string(itemJSON))
		}
	}
	for _, unexpected := range []string{
		`"seq_id"`,
		`"speaker"`,
		`"user_open_id"`,
		`"user_type"`,
		`"user_role"`,
		`"language"`,
		`"start_time_ms"`,
		`"end_time_ms"`,
		`"sequence_id"`,
		`"transcript_item"`,
	} {
		if strings.Contains(string(got), unexpected) {
			t.Errorf("Transcript output should not contain %s, got %s", unexpected, string(got))
		}
	}
	if !strings.Contains(string(got), `"sentence_id":"987654321"`) {
		t.Errorf("Transcript output should contain sentence_id, got %s", string(got))
	}
}

func TestProcessVCRecordingEnded(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	out := runRecordingProcess[VCRecordingEndedOutput](t, eventTypeRecordingEnded, processVCRecordingEnded, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_rec_end_001",
			"event_type": "vc.recording.recording_ended_v1",
			"create_time": "1761782400200"
		},
		"event": {
			"unique_key": "recording_001",
			"source": "recording_bean",
			"object_type": "minutes",
			"object_id": "minute_token_001"
		}
	}`)

	if out.Type != eventTypeRecordingEnded {
		t.Errorf("Type = %q", out.Type)
	}
	if out.UniqueKey != "recording_001" || out.Source != "recording_bean" {
		t.Errorf("UniqueKey/Source = %q/%q", out.UniqueKey, out.Source)
	}
	if out.EventTime != recordingTestEventTime(1761782400200) {
		t.Errorf("EventTime = %q", out.EventTime)
	}
}

func TestProcessVCRecordingEnded_DropsObjectMetadata(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	got := runRecordingProcessRaw(t, eventTypeRecordingEnded, processVCRecordingEnded, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_rec_end_001",
			"event_type": "vc.recording.recording_ended_v1",
			"create_time": "1761782400200"
		},
		"event": {
			"unique_key": "recording_001",
			"source": "recording_bean",
			"object_type": "minutes",
			"object_id": "minute_token_001"
		}
	}`)

	if strings.Contains(string(got), "object_type") || strings.Contains(string(got), "object_id") {
		t.Fatalf("ended output should drop object metadata, got %s", string(got))
	}
}

func TestProcessVCRecording_DropsTimestampField(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	got := runRecordingProcessRaw(t, eventTypeRecordingStarted, processVCRecordingStarted, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_rec_start_001",
			"event_type": "vc.recording.recording_started_v1",
			"create_time": "1761782400000"
		},
		"event": {
			"unique_key": "recording_001",
			"source": "recording_bean"
		}
	}`)

	if strings.Contains(string(got), `"timestamp"`) {
		t.Fatalf("recording output should use event_time instead of timestamp, got %s", string(got))
	}
	if !strings.Contains(string(got), `"event_time":"`+recordingTestEventTime(1761782400000)+`"`) {
		t.Fatalf("recording output should include ISO 8601 event_time, got %s", string(got))
	}
}

func TestProcessVCRecording_NonRecordingBeanFiltered(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, tc := range []struct {
		name      string
		eventType string
		process   event.ProcessFunc
		payload   string
	}{
		{
			name:      "started",
			eventType: eventTypeRecordingStarted,
			process:   processVCRecordingStarted,
			payload: `{
				"schema": "2.0",
				"header": {"event_id": "ev_rec_start_001", "event_type": "vc.recording.recording_started_v1"},
				"event": {"unique_key": "recording_001", "source": "software_recording"}
			}`,
		},
		{
			name:      "transcript",
			eventType: eventTypeRecordingTranscriptGenerated,
			process:   processVCRecordingTranscriptGenerated,
			payload: `{
				"schema": "2.0",
				"header": {"event_id": "ev_rec_transcript_001", "event_type": "vc.recording.recording_transcript_generated_v1"},
				"event": {"unique_key": "recording_001", "source": "software_recording", "transcript_items": []}
			}`,
		},
		{
			name:      "ended",
			eventType: eventTypeRecordingEnded,
			process:   processVCRecordingEnded,
			payload: `{
				"schema": "2.0",
				"header": {"event_id": "ev_rec_end_001", "event_type": "vc.recording.recording_ended_v1"},
				"event": {"unique_key": "recording_001", "source": "software_recording"}
			}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runRecordingProcessRaw(t, tc.eventType, tc.process, tc.payload)
			if got != nil {
				t.Fatalf("non-recording_bean event should be filtered, got %s", string(got))
			}
		})
	}
}

func TestProcessVCRecording_MalformedPayloadPassthrough(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, tc := range []struct {
		name      string
		eventType string
		process   event.ProcessFunc
	}{
		{name: "started", eventType: eventTypeRecordingStarted, process: processVCRecordingStarted},
		{name: "transcript", eventType: eventTypeRecordingTranscriptGenerated, process: processVCRecordingTranscriptGenerated},
		{name: "ended", eventType: eventTypeRecordingEnded, process: processVCRecordingEnded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := &event.RawEvent{
				EventType: tc.eventType,
				Payload:   json.RawMessage(`not json`),
				Timestamp: time.Now(),
			}
			got, err := tc.process(context.Background(), nil, raw, nil)
			if err != nil {
				t.Fatalf("Process should swallow parse errors, got %v", err)
			}
			if string(got) != "not json" {
				t.Errorf("malformed fallback output = %q, want original bytes", string(got))
			}
		})
	}
}

func TestVCRecording_PreConsumeSubscriptionLifecycle(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, tc := range []struct {
		eventType string
	}{
		{eventTypeRecordingStarted},
		{eventTypeRecordingTranscriptGenerated},
		{eventTypeRecordingEnded},
	} {
		t.Run(tc.eventType, func(t *testing.T) {
			def, ok := event.Lookup(tc.eventType)
			if !ok {
				t.Fatalf("%s should be registered via Keys()", tc.eventType)
			}

			type call struct {
				method string
				path   string
				body   any
			}
			var calls []call
			rt := &stubAPIClient{
				callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
					calls = append(calls, call{method: method, path: path, body: body})
					return json.RawMessage(`{"code":0,"msg":"success","data":{}}`), nil
				},
			}

			cleanup, err := def.PreConsume(context.Background(), rt, nil)
			if err != nil {
				t.Fatalf("PreConsume error: %v", err)
			}
			if cleanup == nil {
				t.Fatal("cleanup must not be nil")
			}
			if len(calls) != 1 {
				t.Fatalf("calls after subscribe = %d, want 1", len(calls))
			}
			if calls[0].method != "POST" || calls[0].path != pathRecordingSubscribe {
				t.Fatalf("subscribe call = %+v", calls[0])
			}
			assertSubscriptionRequest(t, calls[0].body, tc.eventType)

			cleanup()
			if len(calls) != 2 {
				t.Fatalf("calls after cleanup = %d, want 2", len(calls))
			}
			if calls[1].method != "POST" || calls[1].path != pathRecordingUnsubscribe {
				t.Fatalf("unsubscribe call = %+v", calls[1])
			}
			assertSubscriptionRequest(t, calls[1].body, tc.eventType)
		})
	}
}

func runRecordingProcess[T any](t *testing.T, eventType string, process event.ProcessFunc, payload string) T {
	t.Helper()
	got := runRecordingProcessRaw(t, eventType, process, payload)
	if got == nil {
		t.Fatal("Process output is nil")
	}
	var out T
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid JSON: %v\nraw=%s", err, string(got))
	}
	return out
}

func runRecordingProcessRaw(t *testing.T, eventType string, process event.ProcessFunc, payload string) json.RawMessage {
	t.Helper()
	raw := &event.RawEvent{
		EventType: eventType,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := process(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	return got
}

func recordingTestEventTime(millis int64) string {
	return time.UnixMilli(millis).Local().Format(time.RFC3339)
}
