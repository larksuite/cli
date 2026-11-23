// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/processing"
)

// VCParticipantMeetingEndedOutput is the flattened shape for vc.meeting.participant_meeting_ended_v1.
type VCParticipantMeetingEndedOutput struct {
	Type            string `json:"type"                      desc:"Event type; always vc.meeting.participant_meeting_ended_v1"`
	EventID         string `json:"event_id,omitempty"        desc:"Globally unique event ID; safe for deduplication"`
	Timestamp       string `json:"timestamp,omitempty"       desc:"Event delivery time (ms timestamp string); taken from header.create_time when present"                                                                 kind:"timestamp_ms"`
	MeetingID       string `json:"meeting_id,omitempty"      desc:"Meeting ID"                                                                                                                                                kind:"meeting_id"`
	Topic           string `json:"topic,omitempty"           desc:"Meeting topic"`
	MeetingNo       string `json:"meeting_no,omitempty"      desc:"Meeting number"`
	StartTime       string `json:"start_time,omitempty"      desc:"Meeting start time in RFC3339, converted to the local timezone"`
	EndTime         string `json:"end_time,omitempty"        desc:"Meeting end time in RFC3339, converted to the local timezone"`
	CalendarEventID string `json:"calendar_event_id,omitempty" desc:"Calendar event ID associated with the meeting"`
}

type participantMeetingEndedEvent struct {
	Meeting struct {
		ID              string `json:"id"`
		Topic           string `json:"topic"`
		MeetingNo       string `json:"meeting_no"`
		StartTime       string `json:"start_time"`
		EndTime         string `json:"end_time"`
		CalendarEventID string `json:"calendar_event_id"`
	} `json:"meeting"`
}

func processVCParticipantMeetingEnded(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	body, ok := decodeEventBody[participantMeetingEndedEvent](raw)
	if !ok {
		return nil, processing.DropMalformed(raw.EventType)
	}

	meeting := body.Meeting
	out := &VCParticipantMeetingEndedOutput{
		Type:            raw.EventType,
		EventID:         raw.EventID,
		Timestamp:       raw.SourceTime,
		MeetingID:       meeting.ID,
		Topic:           meeting.Topic,
		MeetingNo:       meeting.MeetingNo,
		StartTime:       unixSecondsToLocalRFC3339(meeting.StartTime),
		EndTime:         unixSecondsToLocalRFC3339(meeting.EndTime),
		CalendarEventID: meeting.CalendarEventID,
	}
	return json.Marshal(out)
}
