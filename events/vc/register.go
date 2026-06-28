// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package vc registers VC-domain EventKeys.
package vc

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const (
	eventTypeMeetingEnded                 = "vc.meeting.participant_meeting_ended_v1"
	eventTypeNoteGenerated                = "vc.note.generated_v1"
	eventTypeRecordingStarted             = "vc.recording.recording_started_v1"
	eventTypeRecordingTranscriptGenerated = "vc.recording.recording_transcript_generated_v1"
	eventTypeRecordingEnded               = "vc.recording.recording_ended_v1"
	eventTypeBotMeetingInvited            = "vc.bot.meeting_invited_v1"
	eventTypeBotMeetingEvent              = "vc.bot.meeting_event_v1"
	eventTypeBotMeetingEnded              = "vc.bot.meeting_ended_v1"

	pathMeetingSubscribe     = "/open-apis/vc/v1/meetings/subscription"
	pathMeetingUnsubscribe   = "/open-apis/vc/v1/meetings/unsubscription"
	pathNoteSubscribe        = "/open-apis/vc/v1/notes/subscription"
	pathNoteUnsubscribe      = "/open-apis/vc/v1/notes/unsubscription"
	pathRecordingSubscribe   = "/open-apis/vc/v1/recordings/subscription"
	pathRecordingUnsubscribe = "/open-apis/vc/v1/recordings/unsubscription"

	pathNoteDetailFmt = "/open-apis/vc/v1/notes/%s"
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
		{
			Key:         eventTypeNoteGenerated,
			DisplayName: "Note generated",
			Description: "Triggered when a note has been generated",
			EventType:   eventTypeNoteGenerated,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(VCNoteGeneratedOutput{})},
			},
			Process:    processVCNoteGenerated,
			PreConsume: subscriptionPreConsume(eventTypeNoteGenerated, pathNoteSubscribe, pathNoteUnsubscribe),
			Scopes:     []string{"vc:note:read"},
			AuthTypes: []string{
				"user",
			},
			RequiredConsoleEvents: []string{eventTypeNoteGenerated},
		},
		{
			Key:         eventTypeRecordingStarted,
			DisplayName: "Recording started",
			Description: "Triggered when a recording_bean recording starts; only generated when connected to Feishu software.",
			EventType:   eventTypeRecordingStarted,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(VCRecordingStartedOutput{})},
			},
			Process:    processVCRecordingStarted,
			PreConsume: subscriptionPreConsume(eventTypeRecordingStarted, pathRecordingSubscribe, pathRecordingUnsubscribe),
			Scopes:     []string{"vc:recording:read"},
			AuthTypes: []string{
				"user",
			},
			RequiredConsoleEvents: []string{eventTypeRecordingStarted},
		},
		{
			Key:         eventTypeRecordingTranscriptGenerated,
			DisplayName: "Recording transcript generated",
			Description: "Triggered when recording_bean transcript items are generated; only generated when connected to Feishu software.",
			EventType:   eventTypeRecordingTranscriptGenerated,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(VCRecordingTranscriptGeneratedOutput{})},
			},
			Process:    processVCRecordingTranscriptGenerated,
			PreConsume: subscriptionPreConsume(eventTypeRecordingTranscriptGenerated, pathRecordingSubscribe, pathRecordingUnsubscribe),
			Scopes:     []string{"vc:recording:read"},
			AuthTypes: []string{
				"user",
			},
			RequiredConsoleEvents: []string{eventTypeRecordingTranscriptGenerated},
		},
		{
			Key:         eventTypeRecordingEnded,
			DisplayName: "Recording ended",
			Description: "Triggered when a recording_bean recording ends and uploads successfully; only generated when connected to Feishu software.",
			EventType:   eventTypeRecordingEnded,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(VCRecordingEndedOutput{})},
			},
			Process:    processVCRecordingEnded,
			PreConsume: subscriptionPreConsume(eventTypeRecordingEnded, pathRecordingSubscribe, pathRecordingUnsubscribe),
			Scopes:     []string{"vc:recording:read"},
			AuthTypes: []string{
				"user",
			},
			RequiredConsoleEvents: []string{eventTypeRecordingEnded},
		},
		{
			Key:         eventTypeBotMeetingInvited,
			DisplayName: "Bot meeting invited",
			Description: "Triggered when the bot is invited to a meeting; bot-observed event that does not create a user-side VC subscription",
			EventType:   eventTypeBotMeetingInvited,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(VCBotEventOutput{})},
			},
			Process:               processVCBotMeetingInvited,
			AuthTypes:             []string{"bot"},
			RequiredConsoleEvents: []string{eventTypeBotMeetingInvited},
		},
		{
			Key:         eventTypeBotMeetingEvent,
			DisplayName: "Bot meeting event",
			Description: "Triggered when the bot observes activity in a meeting; keeps the raw bot payload and extracts stable activity fields",
			EventType:   eventTypeBotMeetingEvent,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(VCBotEventOutput{})},
			},
			Process:               processVCBotMeetingEvent,
			AuthTypes:             []string{"bot"},
			RequiredConsoleEvents: []string{eventTypeBotMeetingEvent},
		},
		{
			Key:         eventTypeBotMeetingEnded,
			DisplayName: "Bot meeting ended",
			Description: "Triggered when a meeting observed by the bot has ended; distinct from user participant or open meeting resource events",
			EventType:   eventTypeBotMeetingEnded,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(VCBotEventOutput{})},
			},
			Process:               processVCBotMeetingEnded,
			AuthTypes:             []string{"bot"},
			RequiredConsoleEvents: []string{eventTypeBotMeetingEnded},
		},
	}
}
