// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

func TestMain(m *testing.M) {
	for _, k := range Keys() {
		event.RegisterKey(k)
	}
	os.Exit(m.Run())
}

func TestVCKeys_ProcessedMeetingEndedRegistered(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	def, ok := event.Lookup(eventTypeMeetingEnded)
	if !ok {
		t.Fatalf("%s should be registered via Keys()", eventTypeMeetingEnded)
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
}

func TestProcessVCParticipantMeetingEnded(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_vc_end_001",
			"event_type": "vc.meeting.participant_meeting_ended_v1",
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
	out := runMeetingEnded(t, payload)

	if out.Type != eventTypeMeetingEnded {
		t.Errorf("Type = %q", out.Type)
	}
	if out.EventID != "ev_vc_end_001" {
		t.Errorf("EventID = %q", out.EventID)
	}
	if out.Timestamp != "1608725989000" {
		t.Errorf("Timestamp = %q", out.Timestamp)
	}
	if out.MeetingID != "6911188411934433028" {
		t.Errorf("MeetingID = %q", out.MeetingID)
	}
	if out.Topic != "my meeting" || out.MeetingNo != "235812466" {
		t.Errorf("Topic/MeetingNo = %q/%q", out.Topic, out.MeetingNo)
	}
	if out.CalendarEventID != "efa67a98-06a8-4df5-8559-746c8f4477ef_0" {
		t.Errorf("CalendarEventID = %q", out.CalendarEventID)
	}
	if want := time.Unix(1608883322, 0).Local().Format(time.RFC3339); out.StartTime != want {
		t.Errorf("StartTime = %q, want %q", out.StartTime, want)
	}
	if want := time.Unix(1608883899, 0).Local().Format(time.RFC3339); out.EndTime != want {
		t.Errorf("EndTime = %q, want %q", out.EndTime, want)
	}
}

func TestProcessVCParticipantMeetingEnded_InvalidMeetingTimes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_vc_end_002",
			"event_type": "vc.meeting.participant_meeting_ended_v1",
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
	out := runMeetingEnded(t, payload)
	if out.StartTime != "" || out.EndTime != "" {
		t.Errorf("StartTime/EndTime = %q/%q, want empty strings", out.StartTime, out.EndTime)
	}
}

func TestProcessVCParticipantMeetingEnded_MalformedPayload(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	raw := &event.RawEvent{
		EventType: eventTypeMeetingEnded,
		Payload:   json.RawMessage(`not json`),
		Timestamp: time.Now(),
	}
	got, err := processVCParticipantMeetingEnded(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process should swallow parse errors, got %v", err)
	}
	if string(got) != "not json" {
		t.Errorf("malformed fallback output = %q, want original bytes", string(got))
	}
}

func TestVCParticipantMeetingEnded_PreConsumeSubscriptionLifecycle(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	def, ok := event.Lookup("vc.meeting.participant_meeting_ended_v1")
	if !ok {
		t.Fatal("vc.meeting.participant_meeting_ended_v1 should be registered via Keys()")
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
	assertSubscriptionRequest(t, calls[0].body, eventTypeMeetingEnded)

	cleanup()
	if len(calls) != 2 {
		t.Fatalf("calls after cleanup = %d, want 2", len(calls))
	}
	if calls[1].method != "POST" || calls[1].path != pathMeetingUnsubscribe {
		t.Fatalf("unsubscribe call = %+v", calls[1])
	}
	assertSubscriptionRequest(t, calls[1].body, eventTypeMeetingEnded)
}

func runMeetingEnded(t *testing.T, payload string) VCParticipantMeetingEndedOutput {
	t.Helper()
	raw := &event.RawEvent{
		EventType: eventTypeMeetingEnded,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := processVCParticipantMeetingEnded(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	var out VCParticipantMeetingEndedOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid VCParticipantMeetingEndedOutput JSON: %v\nraw=%s", err, string(got))
	}
	return out
}
