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
		eventTypeBotMeetingEvent,
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

func TestProcessVCBotEvents_StableFieldsAndRawEvent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cases := []struct {
		name       string
		eventType  string
		process    event.ProcessFunc
		payload    string
		want       VCBotEventOutput
		wantEmojis []string
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
			name:      "meeting event",
			eventType: eventTypeBotMeetingEvent,
			process:   processVCBotMeetingEvent,
			payload: `{
				"schema": "2.0",
				"header": {
					"event_id": "ev_activity",
					"event_type": "vc.bot.meeting_event_v1",
					"create_time": "1776409469274"
				},
				"event": {
					"meeting_no": "987654321",
					"activity_event_type": "chat_message",
					"chat_messages": [
						{"message_type": 3, "reaction_type": {"emoji_type": "JIAYI"}},
						{"message_type": 3, "chat_emoji_types": ["OK", "JIAYI"]}
					]
				}
			}`,
			want: VCBotEventOutput{
				Type:              eventTypeBotMeetingEvent,
				EventID:           "ev_activity",
				Timestamp:         "1776409469274",
				MeetingNo:         "987654321",
				ActivityEventType: "chat_message",
			},
			wantEmojis: []string{"JIAYI", "OK"},
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
			out := runBotEventProcess(t, tc.eventType, tc.process, tc.payload)
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
			if !reflect.DeepEqual(out.ChatEmojiTypes, tc.wantEmojis) {
				t.Errorf("ChatEmojiTypes = %v, want %v", out.ChatEmojiTypes, tc.wantEmojis)
			}
			if len(out.RawEvent) == 0 {
				t.Fatal("RawEvent must be preserved")
			}
			var raw map[string]any
			if err := json.Unmarshal(out.RawEvent, &raw); err != nil {
				t.Fatalf("RawEvent is not valid JSON: %v", err)
			}
			if raw["schema"] != "2.0" {
				t.Errorf("RawEvent schema = %v, want 2.0", raw["schema"])
			}
		})
	}
}

func TestProcessVCBotMeetingEvent_MalformedPassthrough(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	raw := &event.RawEvent{
		EventID:   "ev_bad",
		EventType: eventTypeBotMeetingEvent,
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

func runBotEventProcess(t *testing.T, eventType string, process event.ProcessFunc, payload string) VCBotEventOutput {
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
	return out
}
