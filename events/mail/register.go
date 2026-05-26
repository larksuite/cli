// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package mail registers mail-domain EventKeys.
package mail

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

// Keys returns all mail-domain EventKey definitions.
// MUST stay in sync with shortcuts/mail/mail_watch.go:98 Scopes field
// (single source of truth: identical 7 items, same order).
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         mailEventType,
			DisplayName: "Receive mail",
			Description: "Receive new mail events for one or more mailboxes (comma-separated --param mailbox)",
			EventType:   mailEventType,
			Params: []event.ParamDef{
				{
					Name:        "mailbox",
					Type:        event.ParamString,
					Required:    false,
					Default:     "me",
					Description: "mailbox email address(es); comma-separated for multi (e.g. alice@x.com,bob@x.com); default 'me' for the primary mailbox of the logged-in user",
				},
			},
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(MailMessageReceivedOutput{})},
			},
			Process:    processMailMessageReceived,
			PreConsume: mailMessageReceivedPreConsume,
			// MUST stay in sync with shortcuts/mail/mail_watch.go:98 (single
			// source of truth; same 7 items, same order). mail +watch and
			// this EventKey require the exact same scope set.
			Scopes: []string{
				"mail:event",
				"mail:user_mailbox.event.mail_address:read",
				"mail:user_mailbox:readonly",
				"mail:user_mailbox.message:readonly",
				"mail:user_mailbox.message.address:read",
				"mail:user_mailbox.message.subject:read",
				"mail:user_mailbox.message.body:read",
			},
			AuthTypes:             []string{"user"},
			RequiredConsoleEvents: []string{mailEventType},
		},
	}
}
