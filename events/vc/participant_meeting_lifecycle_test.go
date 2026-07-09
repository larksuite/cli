// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

func TestVCKeys_ProcessedMeetingLifecycleRegistered(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, tc := range []struct {
		eventType  string
		schemaType reflect.Type
	}{
		{eventTypeMeetingStarted, reflect.TypeOf(VCParticipantMeetingStartedOutput{})},
		{eventTypeMeetingJoined, reflect.TypeOf(VCParticipantMeetingJoinedOutput{})},
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
			if len(def.Scopes) != 1 || def.Scopes[0] != "vc:meeting.meetingevent:read" {
				t.Errorf("Scopes = %v", def.Scopes)
			}
			if len(def.AuthTypes) != 1 || def.AuthTypes[0] != "user" {
				t.Errorf("AuthTypes = %v", def.AuthTypes)
			}
			if len(def.RequiredConsoleEvents) != 1 || def.RequiredConsoleEvents[0] != tc.eventType {
				t.Errorf("RequiredConsoleEvents = %v", def.RequiredConsoleEvents)
			}
			if def.Schema.Custom.Type != tc.schemaType {
				t.Errorf("Custom schema Type = %v, want %v", def.Schema.Custom.Type, tc.schemaType)
			}
		})
	}
}

func TestProcessVCParticipantMeetingLifecycle(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, tc := range []struct {
		name      string
		eventType string
		process   event.ProcessFunc
	}{
		{
			name:      "started",
			eventType: eventTypeMeetingStarted,
			process:   processVCParticipantMeetingStarted,
		},
		{
			name:      "joined",
			eventType: eventTypeMeetingJoined,
			process:   processVCParticipantMeetingJoined,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{
				"schema": "2.0",
				"header": {
					"event_id": "ev_vc_lifecycle_001",
					"event_type": "` + tc.eventType + `",
					"create_time": "1608725989000",
					"app_id": "cli_test"
				},
				"event": {
					"meeting": {
						"id": "6911188411934433028",
						"topic": "my meeting",
						"meeting_no": "235812466",
						"start_time": "1608883322",
						"end_time": "1608883899",
						"calendar_event_id": "efa67a98-06a8-4df5-8559-746c8f4477ef_0"
					}
				}
			}`
			out := runMeetingLifecycleMap(t, tc.eventType, tc.process, payload)

			if out["type"] != tc.eventType {
				t.Errorf("type = %q", out["type"])
			}
			if out["event_id"] != "ev_vc_lifecycle_001" {
				t.Errorf("event_id = %q", out["event_id"])
			}
			if out["timestamp"] != "1608725989000" {
				t.Errorf("timestamp = %q", out["timestamp"])
			}
			if out["meeting_id"] != "6911188411934433028" {
				t.Errorf("meeting_id = %q", out["meeting_id"])
			}
			if out["topic"] != "my meeting" || out["meeting_no"] != "235812466" {
				t.Errorf("topic/meeting_no = %q/%q", out["topic"], out["meeting_no"])
			}
			if out["calendar_event_id"] != "efa67a98-06a8-4df5-8559-746c8f4477ef_0" {
				t.Errorf("calendar_event_id = %q", out["calendar_event_id"])
			}
			if want := time.Unix(1608883322, 0).Local().Format(time.RFC3339); out["start_time"] != want {
				t.Errorf("start_time = %q, want %q", out["start_time"], want)
			}
			if _, hasEndTime := out["end_time"]; hasEndTime {
				t.Error("end_time should not be present in started/joined output")
			}
		})
	}
}

func TestProcessVCParticipantMeetingLifecycle_InvalidMeetingTimes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, tc := range []struct {
		name      string
		eventType string
		process   event.ProcessFunc
	}{
		{"started", eventTypeMeetingStarted, processVCParticipantMeetingStarted},
		{"joined", eventTypeMeetingJoined, processVCParticipantMeetingJoined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{
				"schema": "2.0",
				"header": {
					"event_id": "ev_vc_lifecycle_002",
					"event_type": "` + tc.eventType + `",
					"create_time": "1608725989001"
				},
				"event": {
					"meeting": {
						"id": "meeting_invalid_time",
						"start_time": "bad",
						"end_time": ""
					}
				}
			}`
			out := runMeetingLifecycleRaw(t, tc.eventType, tc.process, payload)
			switch tc.eventType {
			case eventTypeMeetingStarted:
				var started VCParticipantMeetingStartedOutput
				if err := json.Unmarshal(out, &started); err != nil {
					t.Fatalf("Process output is not valid started JSON: %v\nraw=%s", err, string(out))
				}
				if started.StartTime != "" {
					t.Errorf("StartTime = %q, want empty string", started.StartTime)
				}
			case eventTypeMeetingJoined:
				var joined VCParticipantMeetingJoinedOutput
				if err := json.Unmarshal(out, &joined); err != nil {
					t.Fatalf("Process output is not valid joined JSON: %v\nraw=%s", err, string(out))
				}
				if joined.StartTime != "" {
					t.Errorf("StartTime = %q, want empty string", joined.StartTime)
				}
			}
		})
	}
}

func TestProcessVCParticipantMeetingLifecycle_MalformedPayload(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, tc := range []struct {
		name      string
		eventType string
		process   event.ProcessFunc
	}{
		{"started", eventTypeMeetingStarted, processVCParticipantMeetingStarted},
		{"joined", eventTypeMeetingJoined, processVCParticipantMeetingJoined},
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

func TestVCParticipantMeetingLifecycle_PreConsumeSubscriptionLifecycle(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, eventType := range []string{eventTypeMeetingStarted, eventTypeMeetingJoined} {
		t.Run(eventType, func(t *testing.T) {
			def, ok := event.Lookup(eventType)
			if !ok {
				t.Fatalf("%s should be registered via Keys()", eventType)
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
			if calls[0].method != "POST" || calls[0].path != pathMeetingSubscribe {
				t.Fatalf("subscribe call = %+v", calls[0])
			}
			assertSubscriptionRequest(t, calls[0].body, eventType)

			cleanup()
			if len(calls) != 2 {
				t.Fatalf("calls after cleanup = %d, want 2", len(calls))
			}
			if calls[1].method != "POST" || calls[1].path != pathMeetingUnsubscribe {
				t.Fatalf("unsubscribe call = %+v", calls[1])
			}
			assertSubscriptionRequest(t, calls[1].body, eventType)
		})
	}
}

func runMeetingLifecycleMap(t *testing.T, eventType string, process event.ProcessFunc, payload string) map[string]string {
	t.Helper()
	got := runMeetingLifecycleRaw(t, eventType, process, payload)
	if got == nil {
		t.Fatal("Process output is nil")
	}
	var out map[string]string
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid flat JSON object: %v\nraw=%s", err, string(got))
	}
	return out
}

func runMeetingLifecycleRaw(t *testing.T, eventType string, process event.ProcessFunc, payload string) json.RawMessage {
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
