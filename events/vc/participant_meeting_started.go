// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/internal/event"
)

// VCParticipantMeetingStartedOutput is the flattened shape for vc.meeting.participant_meeting_started_v1.
type VCParticipantMeetingStartedOutput struct {
	Type            string `json:"type"                      desc:"Event type; always vc.meeting.participant_meeting_started_v1"`
	EventID         string `json:"event_id,omitempty"        desc:"Globally unique event ID; safe for deduplication"`
	Timestamp       string `json:"timestamp,omitempty"       desc:"Event delivery time (ms timestamp string); taken from header.create_time when present" kind:"timestamp_ms"`
	MeetingID       string `json:"meeting_id,omitempty"      desc:"Meeting ID" kind:"meeting_id"`
	Topic           string `json:"topic,omitempty"           desc:"Meeting topic"`
	MeetingNo       string `json:"meeting_no,omitempty"      desc:"Meeting number"`
	StartTime       string `json:"start_time,omitempty"      desc:"Meeting start time in RFC3339, converted to the local timezone"`
	CalendarEventID string `json:"calendar_event_id,omitempty" desc:"Calendar event ID associated with the meeting"`
}

func processVCParticipantMeetingStarted(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event struct {
			Meeting struct {
				ID              string `json:"id"`
				Topic           string `json:"topic"`
				MeetingNo       string `json:"meeting_no"`
				StartTime       string `json:"start_time"`
				CalendarEventID string `json:"calendar_event_id"`
			} `json:"meeting"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	meeting := envelope.Event.Meeting
	out := &VCParticipantMeetingStartedOutput{
		Type:            envelope.Header.EventType,
		EventID:         envelope.Header.EventID,
		Timestamp:       envelope.Header.CreateTime,
		MeetingID:       meeting.ID,
		Topic:           meeting.Topic,
		MeetingNo:       meeting.MeetingNo,
		StartTime:       unixSecondsToLocalRFC3339(meeting.StartTime),
		CalendarEventID: meeting.CalendarEventID,
	}
	if out.Type == "" {
		out.Type = raw.EventType
	}
	return json.Marshal(out)
}
