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
			Description: "接收 IM 消息（text/post/image/file/audio/media/sticker/interactive/share_chat/share_user/system 所有类型）",
			EventType:   "im.message.receive_v1",
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(ImMessageReceiveOutput{})},
			},
			Process:               processImMessageReceive,
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
