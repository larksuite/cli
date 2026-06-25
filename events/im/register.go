// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package im registers IM-domain EventKeys.
package im

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

// Keys returns all IM-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	out := []event.KeyDefinition{
		{
			Key:         "im.message.receive_v1",
			DisplayName: "Receive message",
			Description: "Receive IM messages",
			EventType:   "im.message.receive_v1",
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(ImMessageReceiveOutput{})},
			},
			Process: processImMessageReceive,
			// Narrowest grant; kept single-element since MissingScopes uses AND semantics.
			Scopes:                []string{"im:message.p2p_msg:readonly"},
			AuthTypes:             []string{"bot"},
			RequiredConsoleEvents: []string{"im.message.receive_v1"},
		},
		{
			Key:              "card.action.trigger",
			DisplayName:      "Card action",
			Description:      "Triggered when a user interacts with an interactive card (button click, form submit, dropdown select, etc.). Output includes: token (valid 30 min, max 2 updates), action details (tag, value, name, form_value), and card_content (original card in userDSL text format, auto-fetched at consume time). To update the card: parse card_content to understand the current state, construct the new card JSON, then call `lark-cli api POST /open-apis/interactive/v1/card/update` with the token (see lark-im-card-action-reply.md).",
			EventType:        "card.action.trigger",
			SubscriptionType: event.SubTypeCallback,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(CardActionTriggerOutput{})},
			},
			Process:               processCardAction,
			Scopes:                []string{"im:message:readonly"},
			AuthTypes:             []string{"bot"},
			SingleConsumer:        true,
			RequiredConsoleEvents: []string{"card.action.trigger"},
		},
	}

	for _, rk := range nativeIMKeys {
		out = append(out, event.KeyDefinition{
			Key:         rk.key,
			DisplayName: rk.title,
			Description: rk.description,
			EventType:   rk.key,
			Schema: event.SchemaDef{
				Native:         &event.SchemaSpec{Type: rk.bodyType},
				FieldOverrides: rk.fieldOverrides,
			},
			Scopes:                rk.scopes,
			AuthTypes:             []string{"bot"},
			RequiredConsoleEvents: []string{rk.key},
		})
	}

	return out
}
