// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package whiteboard registers Board-domain EventKeys.
package whiteboard

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/schemas"
)

// eventTypeWhiteboardUpdated is the OAPI event type for whiteboard content updates.
const eventTypeWhiteboardUpdated = "board.whiteboard.updated_v1"

// Keys returns all Board-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         eventTypeWhiteboardUpdated,
			DisplayName: "Whiteboard updated",
			Description: "Pushed when the whiteboard content is updated.",
			EventType:   eventTypeWhiteboardUpdated,
			Params: []event.ParamDef{
				{
					Name:        "whiteboard_id",
					Type:        event.ParamString,
					Required:    true,
					Description: "Whiteboard id to subscribe; subscription is per-whiteboard.",
				},
			},
			Schema: event.SchemaDef{
				Native: &event.SchemaSpec{Type: reflect.TypeOf(BoardWhiteboardUpdatedV1Data{})},
				FieldOverrides: map[string]schemas.FieldMeta{
					"/event/whiteboard_id":           {Kind: "whiteboard_id", Description: "whiteboard id to subscribe"},
					"/event/operator_ids/*/open_id":  {Kind: "open_id"},
					"/event/operator_ids/*/union_id": {Kind: "union_id"},
					"/event/operator_ids/*/user_id":  {Kind: "user_id"},
				},
			},
			PreConsume:            whiteboardSubscriptionPreConsume(eventTypeWhiteboardUpdated),
			Scopes:                []string{"board:whiteboard:node:read"},
			AuthTypes:             []string{"user", "bot"},
			RequiredConsoleEvents: []string{eventTypeWhiteboardUpdated},
		},
	}
}
