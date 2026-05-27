// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package vc registers VC-domain EventKeys.
package vc

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const (
	eventTypeMeetingEnded  = "vc.meeting.participant_meeting_ended_v1"
	eventTypeNoteGenerated = "vc.note.generated_v1"

	pathMeetingSubscribe   = "/open-apis/vc/v1/meetings/subscription"
	pathMeetingUnsubscribe = "/open-apis/vc/v1/meetings/unsubscription"
	pathNoteSubscribe      = "/open-apis/vc/v1/notes/subscription"
	pathNoteUnsubscribe    = "/open-apis/vc/v1/notes/unsubscription"
)

// Keys returns all VC-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         eventTypeMeetingEnded,
			DisplayName: "Participant meeting ended",
			Description: "Triggered when a meeting the current user participates in has ended",
			EventType:   eventTypeMeetingEnded,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(VCParticipantMeetingEndedOutput{})},
			},
			Process:    processVCParticipantMeetingEnded,
			PreConsume: subscriptionPreConsume(eventTypeMeetingEnded, pathMeetingSubscribe, pathMeetingUnsubscribe),
			Scopes:     []string{"vc:meeting.meetingevent:read"},
			AuthTypes: []string{
				"user",
			},
			RequiredConsoleEvents: []string{eventTypeMeetingEnded},
		},
	}
}
