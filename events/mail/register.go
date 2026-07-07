// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package mail registers Mail-domain EventKeys.
package mail

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const EventTypeMessageReceived = "mail.user_mailbox.event.message_received_v1"

type WatchOutput struct {
	Message map[string]interface{} `json:"message,omitempty" desc:"Fetched mail message payload"`
	Header  map[string]interface{} `json:"header,omitempty"  desc:"Raw event header when msg_format=event"`
	Event   map[string]interface{} `json:"event,omitempty"   desc:"Raw mail event body when msg_format=event"`
}

func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         EventTypeMessageReceived,
			DisplayName: "Mail message received",
			Description: "Receive new mail message events",
			EventType:   EventTypeMessageReceived,
			Params: []event.ParamDef{
				{Name: "mailbox", Type: event.ParamString, Default: "me", Description: "Mailbox to subscribe", SubscriptionKey: true},
				{Name: "format", Type: event.ParamEnum, Default: "data", Description: "Output envelope mode", Values: []event.ParamValue{
					{Value: "json", Desc: "Wrap each event in an ok/data envelope"},
					{Value: "data", Desc: "Emit bare event data"},
				}},
				{Name: "msg_format", Type: event.ParamEnum, Default: "metadata", Description: "Message payload mode", Values: []event.ParamValue{
					{Value: "metadata", Desc: "Fetch metadata and header fields"},
					{Value: "minimal", Desc: "Fetch metadata and emit only IDs/state fields"},
					{Value: "plain_text_full", Desc: "Fetch metadata plus plain-text body"},
					{Value: "full", Desc: "Fetch full message payload"},
					{Value: "event", Desc: "Emit raw event payload without message fetch unless filters require it"},
				}},
				{Name: "label_ids", Type: event.ParamString, Description: "JSON array or comma-separated label IDs to filter"},
				{Name: "folder_ids", Type: event.ParamString, Description: "JSON array or comma-separated folder IDs to filter"},
				{Name: "labels", Type: event.ParamString, Description: "JSON array of label names to resolve before consuming"},
				{Name: "folders", Type: event.ParamString, Description: "JSON array of folder names to resolve before consuming"},
			},
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(WatchOutput{})},
			},
			NormalizeParams:       normalizeWatchParams,
			Match:                 matchWatchMailbox,
			Process:               processWatchEvent,
			PreConsume:            mailboxEventPreConsume(EventTypeMessageReceived),
			Scopes:                []string{"mail:event", "mail:user_mailbox.event.mail_address:read", "mail:user_mailbox:readonly", "mail:user_mailbox.message:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
			AuthTypes:             []string{"user"},
			RequiredConsoleEvents: []string{EventTypeMessageReceived},
		},
	}
}
