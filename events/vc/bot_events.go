// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/internal/event"
)

// VCBotEventOutput is the raw-preserving shape for bot-observed VC events.
type VCBotEventOutput struct {
	Type              string          `json:"type"                         desc:"Event type; one of the supported vc.bot.* keys"`
	EventID           string          `json:"event_id,omitempty"           desc:"Globally unique event ID; safe for deduplication"`
	Timestamp         string          `json:"timestamp,omitempty"          desc:"Event delivery time (ms timestamp string); taken from header.create_time when present" kind:"timestamp_ms"`
	CallID            string          `json:"call_id,omitempty"            desc:"Bot invitation call ID; pass through to vc agent join when present"`
	MeetingNo         string          `json:"meeting_no,omitempty"         desc:"Meeting number from the bot event's declared meeting field"`
	ActivityEventType string          `json:"activity_event_type,omitempty" desc:"First event.meeting_activity_items[].activity_event_type value"`
	RawEvent          json.RawMessage `json:"raw_event,omitempty"          desc:"Original VC bot event payload; authoritative for fields not exposed as stable top-level fields"`
}

func processVCBotMeetingInvited(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	return processVCBotEvent(raw)
}

func processVCBotMeetingEvent(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	return processVCBotEvent(raw)
}

func processVCBotMeetingEnded(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	return processVCBotEvent(raw)
}

type vcBotEventEnvelope struct {
	Header struct {
		EventID    string `json:"event_id"`
		EventType  string `json:"event_type"`
		CreateTime string `json:"create_time"`
	} `json:"header"`
	Event json.RawMessage `json:"event"`
}

type vcBotMeetingInvitedEvent struct {
	CallID  string `json:"call_id"`
	Meeting struct {
		MeetingNo string `json:"meeting_no"`
	} `json:"meeting"`
}

type vcBotMeetingActivityEvent struct {
	MeetingActivityItems []vcBotMeetingActivityItem `json:"meeting_activity_items"`
}

type vcBotMeetingActivityItem struct {
	ActivityEventType string `json:"activity_event_type"`
	Meeting           struct {
		MeetingNo string `json:"meeting_no"`
	} `json:"meeting"`
}

type vcBotMeetingEndedEvent struct {
	MeetingNo string `json:"meeting_no"`
}

func processVCBotEvent(raw *event.RawEvent) (json.RawMessage, error) {
	var envelope vcBotEventEnvelope
	decoder := json.NewDecoder(bytes.NewReader(raw.Payload))
	if err := decoder.Decode(&envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	eventType := envelope.Header.EventType
	if eventType == "" {
		eventType = raw.EventType
	}
	out := &VCBotEventOutput{
		Type:      eventType,
		EventID:   envelope.Header.EventID,
		Timestamp: envelope.Header.CreateTime,
		RawEvent:  append(json.RawMessage(nil), raw.Payload...),
	}
	fillBotEventOutput(eventType, envelope.Event, out)
	return json.Marshal(out)
}

func fillBotEventOutput(eventType string, data json.RawMessage, out *VCBotEventOutput) {
	switch eventType {
	case eventTypeBotMeetingInvited:
		payload, err := decodeBotMeetingInvitedEvent(data)
		if err != nil {
			return
		}
		out.CallID = strings.TrimSpace(payload.CallID)
		out.MeetingNo = strings.TrimSpace(payload.Meeting.MeetingNo)
	case eventTypeBotMeetingActivity:
		payload, err := decodeBotMeetingActivityEvent(data)
		if err != nil {
			return
		}
		out.MeetingNo = botActivityMeetingNo(payload.MeetingActivityItems)
		out.ActivityEventType = botActivityEventType(payload.MeetingActivityItems)
	case eventTypeBotMeetingEnded:
		payload, err := decodeBotMeetingEndedEvent(data)
		if err != nil {
			return
		}
		out.MeetingNo = strings.TrimSpace(payload.MeetingNo)
	}
}

func decodeBotMeetingInvitedEvent(data json.RawMessage) (vcBotMeetingInvitedEvent, error) {
	var payload vcBotMeetingInvitedEvent
	if err := json.Unmarshal(data, &payload); err != nil {
		return vcBotMeetingInvitedEvent{}, err
	}
	return payload, nil
}

func decodeBotMeetingActivityEvent(data json.RawMessage) (vcBotMeetingActivityEvent, error) {
	var payload vcBotMeetingActivityEvent
	if err := json.Unmarshal(data, &payload); err != nil {
		return vcBotMeetingActivityEvent{}, err
	}
	return payload, nil
}

func decodeBotMeetingEndedEvent(data json.RawMessage) (vcBotMeetingEndedEvent, error) {
	var payload vcBotMeetingEndedEvent
	if err := json.Unmarshal(data, &payload); err != nil {
		return vcBotMeetingEndedEvent{}, err
	}
	return payload, nil
}

func botActivityMeetingNo(items []vcBotMeetingActivityItem) string {
	for _, item := range items {
		if meetingNo := strings.TrimSpace(item.Meeting.MeetingNo); meetingNo != "" {
			return meetingNo
		}
	}
	return ""
}

func botActivityEventType(items []vcBotMeetingActivityItem) string {
	for _, item := range items {
		if eventType := strings.TrimSpace(item.ActivityEventType); eventType != "" {
			return eventType
		}
	}
	return ""
}
