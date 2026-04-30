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
