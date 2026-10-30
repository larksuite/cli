// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package drive registers Drive-domain EventKeys.
package drive

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const eventTypeCommentAddV1 = "drive.notice.comment_add_v1"

// Keys returns all Drive-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         eventTypeCommentAddV1,
			DisplayName: "Comment notice added",
			Description: "Triggered when a cloud document comment or reply notifies the current user or bot, for example by @mentioning it.",
			EventType:   eventTypeCommentAddV1,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(CommentAddOutput{})},
			},
			Process:               processCommentAdd,
			PreConsume:            commentSubscriptionPreConsume,
			Scopes:                []string{"docs:document.comment:read"},
			AuthTypes:             []string{"user", "bot"},
			RequiredConsoleEvents: []string{eventTypeCommentAddV1},
		},
	}
}
