// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package mail registers Mail-domain EventKeys.
package mail

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const EventTypeMessageReceived = "mail.user_mailbox.event.message_received_v1"

// Keys returns all Mail-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         EventTypeMessageReceived,
			DisplayName: "Mail message received",
			Description: "Triggered when the subscribed mailbox receives a new message.",
			EventType:   EventTypeMessageReceived,
			Params: []event.ParamDef{
				{
					Name:            "mailbox",
					Type:            event.ParamString,
					Default:         "me",
					Description:     "Mailbox address or me; me is normalized to the primary email address before subscription.",
					SubscriptionKey: true,
				},
				{
					Name:        "msg_format",
					Type:        event.ParamEnum,
					Default:     "metadata",
					Description: "Output payload mode.",
					Values: []event.ParamValue{
						{Value: "event", Desc: "raw V2 event envelope"},
						{Value: "minimal", Desc: "message ids, folder, labels, date, and state only"},
						{Value: "metadata", Desc: "message metadata"},
						{Value: "plain_text_full", Desc: "message metadata plus plain-text body"},
						{Value: "full", Desc: "full message payload"},
					},
				},
				{Name: "folder_ids", Type: event.ParamString, Description: "JSON array of folder IDs."},
				{Name: "folders", Type: event.ParamString, Description: "JSON array of folder names."},
				{Name: "label_ids", Type: event.ParamString, Description: "JSON array of label IDs."},
				{Name: "labels", Type: event.ParamString, Description: "JSON array of label names."},
			},
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(MailMessageReceivedOutput{})},
			},
			NormalizeParams:       normalizeMailMessageReceivedParams,
			Match:                 matchMailMessageReceived,
			Process:               processMailMessageReceived,
			PreConsume:            mailSubscriptionPreConsume(EventTypeMessageReceived),
			Scopes:                []string{"mail:event", "mail:user_mailbox.event.mail_address:read", "mail:user_mailbox:readonly", "mail:user_mailbox.message:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
			AuthTypes:             []string{"user"},
			RequiredConsoleEvents: []string{EventTypeMessageReceived},
		},
	}
}
