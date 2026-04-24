// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package im registers IM-domain EventKeys.
//
// im.message.receive_v1 is a processed key: the envelope is flattened and
// @mentions are resolved via convertlib. All other IM event types are
// native (envelope passthrough), with the SDK struct reflected for schemas.
package im

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

// Keys returns all IM-domain EventKey definitions. The aggregator in
// package events feeds these into event.RegisterKey at startup.
func Keys() []event.KeyDefinition {
	out := []event.KeyDefinition{
		{
			Key:         "im.message.receive_v1",
			DisplayName: "Receive message",
			Description: "接收 IM 消息",
			EventType:   "im.message.receive_v1",
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(ImMessageReceiveOutput{})},
			},
			Process: processImMessageReceive,
			// Narrowest grant that lets a bot read incoming p2p messages;
			// broader scopes (im:message, im:message:readonly) cover this
			// too but shouldn't be required up-front. MissingScopes uses
			// AND semantics so we keep this list single-element rather
			// than listing every acceptable substitute.
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
