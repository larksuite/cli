// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package mail registers Mail-domain EventKeys.
package mail

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const MessageReceivedEventKey = "mail.user_mailbox.event.message_received_v1"

func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         MessageReceivedEventKey,
			DisplayName: "Mail message received",
			Description: "Triggered when a mailbox receives a message",
			EventType:   MessageReceivedEventKey,
			Params: []event.ParamDef{
				{
					Name:            "mailbox",
					Type:            event.ParamString,
					Default:         "me",
					Description:     "Mailbox email address or me. This is the only subscription identity parameter.",
					SubscriptionKey: true,
				},
				{
					Name:        "msg_format",
					Type:        event.ParamEnum,
					Default:     "metadata",
					Description: "Message payload mode.",
					Values: []event.ParamValue{
						{Value: "metadata", Desc: "Fetch message metadata for triage or notification output"},
						{Value: "minimal", Desc: "Fetch metadata then emit only IDs and state fields"},
						{Value: "plain_text_full", Desc: "Fetch full plain-text body with metadata"},
						{Value: "full", Desc: "Fetch full message payload"},
						{Value: "event", Desc: "Emit the raw event without fetching message data unless filters require metadata"},
					},
				},
				{Name: "folder_ids", Type: event.ParamString, Description: "JSON array of folder IDs to filter by."},
				{Name: "folders", Type: event.ParamString, Description: "JSON array of folder names or system aliases to resolve and filter by."},
				{Name: "label_ids", Type: event.ParamString, Description: "JSON array of label IDs to filter by."},
				{Name: "labels", Type: event.ParamString, Description: "JSON array of label names or system aliases to resolve and filter by."},
			},
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(MessageReceivedOutput{})},
			},
			NormalizeParams:       normalizeParams,
			PreConsume:            preConsume,
			Match:                 matchMailbox,
			Process:               processMessageReceived,
			Scopes:                mailScopes,
			AuthTypes:             []string{"user"},
			RequiredConsoleEvents: []string{MessageReceivedEventKey},
		},
	}
}

var mailScopes = []string{
	"mail:event",
	"mail:user_mailbox.event.mail_address:read",
	"mail:user_mailbox:readonly",
	"mail:user_mailbox.message:readonly",
	"mail:user_mailbox.message.address:read",
	"mail:user_mailbox.message.subject:read",
	"mail:user_mailbox.message.body:read",
}
