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

func TestVCKeys_BotEventsRegistered(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	for _, eventType := range []string{
		eventTypeBotMeetingInvited,
		eventTypeBotMeetingActivity,
		eventTypeBotMeetingEnded,
	} {
		t.Run(eventType, func(t *testing.T) {
			def, ok := event.Lookup(eventType)
			if !ok {
				t.Fatalf("%s should be registered via Keys()", eventType)
			}
			if def.Schema.Custom == nil {
				t.Error("bot event must set Schema.Custom")
			}
			if def.Schema.Native != nil {
				t.Error("bot event must not set Schema.Native")
			}
			if def.Process == nil {
				t.Error("bot event Process must not be nil")
			}
			if def.PreConsume != nil {
				t.Fatal("bot event must not reuse user-side VC PreConsume subscription")
			}
			if !reflect.DeepEqual(def.AuthTypes, []string{"bot"}) {
				t.Errorf("AuthTypes = %v, want [bot]", def.AuthTypes)
			}
			if !reflect.DeepEqual(def.RequiredConsoleEvents, []string{eventType}) {
				t.Errorf("RequiredConsoleEvents = %v, want [%s]", def.RequiredConsoleEvents, eventType)
			}
		})
	}
}

func TestProcessVCBotEvents_StableFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cases := []struct {
		name      string
		eventType string
		process   event.ProcessFunc
		payload   string
		want      VCBotEventOutput
	}{
		{
			name:      "invited",
			eventType: eventTypeBotMeetingInvited,
			process:   processVCBotMeetingInvited,
			payload: `{
				"schema": "2.0",
				"header": {
					"event_id": "ev_invited",
					"event_type": "vc.bot.meeting_invited_v1",
					"create_time": "1776409469273"
				},
				"event": {
					"call_id": "call_123",
					"meeting": {"meeting_no": "123456789"}
				}
			}`,
			want: VCBotEventOutput{
				Type:      eventTypeBotMeetingInvited,
				EventID:   "ev_invited",
				Timestamp: "1776409469273",
				CallID:    "call_123",
				MeetingNo: "123456789",
			},
		},
		{
			name:      "meeting activity",
			eventType: eventTypeBotMeetingActivity,
			process:   processVCBotMeetingEvent,
			payload: `{
				"schema": "2.0",
				"header": {
					"event_id": "ev_activity",
					"event_type": "vc.bot.meeting_activity_v1",
					"create_time": "1776409469274"
				},
				"event": {
					"meeting_no": "should_not_use",
					"activity_event_type": "should_not_use",
					"chat_messages": [
						{"message_type": 3, "content": "SHOULD_NOT_COLLECT"}
					],
					"meeting_activity_items": [{
						"activity_event_type": "chat_received",
						"meeting": {"meeting_no": "987654321"},
						"chat_received_items": [
							{"message_type": 1, "content": "hello"},
							{"message_type": 3, "content": "JIAYI", "reaction_type": {"emoji_type": "SHOULD_NOT_COLLECT"}},
							{"message_type": 3, "content": "OK"},
							{"message_type": 3, "content": "JIAYI"}
						]
					}, {
						"activity_event_type": "chat_received",
						"meeting": {"meeting_no": "should_not_use"},
						"chat_received_items": [
							{"message_type": 3, "content": "THUMBSUP"}
						]
					}, {
						"activity_event_type": "participant_joined",
						"meeting": {"meeting_no": "should_not_use"},
						"chat_received_items": [
							{"message_type": 3, "content": "SHOULD_NOT_COLLECT"}
						]
					}]
				}
			}`,
			want: VCBotEventOutput{
				Type:              eventTypeBotMeetingActivity,
				EventID:           "ev_activity",
				Timestamp:         "1776409469274",
				MeetingNo:         "987654321",
				ActivityEventType: "chat_received",
			},
		},
		{
			name:      "meeting activity ignores nested reaction details",
			eventType: eventTypeBotMeetingActivity,
			process:   processVCBotMeetingEvent,
			payload: `{
				"schema": "2.0",
				"header": {
					"event_id": "ev_activity_content",
					"event_type": "vc.bot.meeting_activity_v1",
					"create_time": "1776409469276"
				},
				"event": {
					"meeting_activity_items": [{
						"activity_event_type": "chat_received",
						"chat_received_items": [
							{"message_type": 1, "content": "ws test"},
							{"message_type": 3, "content": "OK"}
						],
						"meeting": {"meeting_no": "427607561"}
					}]
				}
			}`,
			want: VCBotEventOutput{
				Type:              eventTypeBotMeetingActivity,
				EventID:           "ev_activity_content",
				Timestamp:         "1776409469276",
				MeetingNo:         "427607561",
				ActivityEventType: "chat_received",
			},
		},
		{
			name:      "ended",
			eventType: eventTypeBotMeetingEnded,
			process:   processVCBotMeetingEnded,
			payload: `{
				"schema": "2.0",
				"header": {
					"event_id": "ev_ended",
					"event_type": "vc.bot.meeting_ended_v1",
					"create_time": "1776409469275"
				},
				"event": {
					"meeting_no": "246801357"
				}
			}`,
			want: VCBotEventOutput{
				Type:      eventTypeBotMeetingEnded,
				EventID:   "ev_ended",
				Timestamp: "1776409469275",
				MeetingNo: "246801357",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, out := runBotEventProcess(t, tc.eventType, tc.process, tc.payload)
			if out.Type != tc.want.Type || out.EventID != tc.want.EventID || out.Timestamp != tc.want.Timestamp {
				t.Errorf("type/event_id/timestamp = %q/%q/%q", out.Type, out.EventID, out.Timestamp)
			}
			if out.CallID != tc.want.CallID {
				t.Errorf("CallID = %q, want %q", out.CallID, tc.want.CallID)
			}
			if out.MeetingNo != tc.want.MeetingNo {
				t.Errorf("MeetingNo = %q, want %q", out.MeetingNo, tc.want.MeetingNo)
			}
			if out.ActivityEventType != tc.want.ActivityEventType {
				t.Errorf("ActivityEventType = %q, want %q", out.ActivityEventType, tc.want.ActivityEventType)
			}
			var row map[string]any
			if err := json.Unmarshal(got, &row); err != nil {
				t.Fatalf("Process output is not valid JSON: %v\nraw=%s", err, string(got))
			}
			if _, ok := row["raw_event"]; ok {
				t.Fatalf("normal bot event output should not include raw_event: %s", string(got))
			}
			payload, ok := row["payload"].(map[string]any)
			if !ok {
				t.Fatalf("normal bot event output should include event-body payload: %s", string(got))
			}
			if _, ok := payload["header"]; ok {
				t.Fatalf("payload should not include the top-level envelope header: %s", string(got))
			}
			if _, ok := payload["schema"]; ok {
				t.Fatalf("payload should not include the top-level envelope schema: %s", string(got))
			}
		})
	}
}

func TestProcessVCBotMeetingEvent_MalformedPassthrough(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	raw := &event.RawEvent{
		EventID:   "ev_bad",
		EventType: eventTypeBotMeetingActivity,
		Payload:   json.RawMessage(`not json`),
		Timestamp: time.Now(),
	}
	got, err := processVCBotMeetingEvent(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("process error: %v", err)
	}
	if string(got) != "not json" {
		t.Fatalf("malformed payload passthrough = %s, want raw payload", string(got))
	}
}

func TestProcessVCBotMeetingEvent_MalformedActivityPayloadKeepsStableEnvelope(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	got, out := runBotEventProcess(t, eventTypeBotMeetingActivity, processVCBotMeetingEvent, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_bad_activity",
			"event_type": "vc.bot.meeting_activity_v1",
			"create_time": "1776409469277"
		},
		"event": {
			"meeting_activity_items": ["not an activity item"]
		}
	}`)
	if out.Type != eventTypeBotMeetingActivity {
		t.Fatalf("Type = %q, want %q", out.Type, eventTypeBotMeetingActivity)
	}
	if out.MeetingNo != "" || out.ActivityEventType != "" {
		t.Fatalf("stable fields = meeting_no:%q activity_event_type:%q, want empty", out.MeetingNo, out.ActivityEventType)
	}
	var row map[string]any
	if err := json.Unmarshal(got, &row); err != nil {
		t.Fatalf("Process output is not valid JSON: %v\nraw=%s", err, string(got))
	}
	if _, ok := row["raw_event"]; ok {
		t.Fatalf("normal bot event output should not include raw_event: %s", string(got))
	}
	if _, ok := row["payload"].(map[string]any); !ok {
		t.Fatalf("normal bot event output should include event-body payload: %s", string(got))
	}
}

func runBotEventProcess(t *testing.T, eventType string, process event.ProcessFunc, payload string) (json.RawMessage, VCBotEventOutput) {
	t.Helper()
	raw := &event.RawEvent{
		EventID:   "raw_" + eventType,
		EventType: eventType,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := process(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("process %s: %v", eventType, err)
	}
	var out VCBotEventOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, string(got))
	}
	return got, out
}
