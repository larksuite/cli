// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package drive registers Drive-domain EventKeys.
package drive

import (
	"reflect"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/schemas"
)

const eventTypeCommentAddV1 = "drive.notice.comment_add_v1"

// bitableCleanupTimeout bounds the unsubscribe call during PreConsume cleanup
// so a stuck OAPI cannot block process shutdown.
const bitableCleanupTimeout = 5 * time.Second

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
		{
			Key:         eventTypeBitableFieldChangedV1,
			DisplayName: "Bitable field changed",
			Description: "Pushed when a field of a subscribed Base (bitable) is added, deleted or edited.",
			EventType:   eventTypeBitableFieldChangedV1,
			Params:      []event.ParamDef{fileTokenParam("Field changes of other Bases are filtered out.")},
			Schema: event.SchemaDef{
				Native: &event.SchemaSpec{Type: reflect.TypeOf(DriveFileBitableFieldChangedV1Data{})},
				FieldOverrides: map[string]schemas.FieldMeta{
					"/event/operator_id/open_id":  {Kind: "open_id"},
					"/event/operator_id/union_id": {Kind: "union_id"},
					"/event/operator_id/user_id":  {Kind: "user_id"},
				},
			},
			Match:                 bitableFileTokenMatch,
			PreConsume:            bitableSubscriptionPreConsume(eventTypeBitableFieldChangedV1),
			Scopes:                []string{"base:field:read"},
			AuthTypes:             []string{"user", "bot"},
			RequiredConsoleEvents: []string{eventTypeBitableFieldChangedV1},
		},
		{
			Key:         eventTypeBitableRecordChangedV1,
			DisplayName: "Bitable record changed",
			Description: "Pushed when a record of a subscribed Base (bitable) is added, deleted or edited. Formula-only changes do not trigger it.",
			EventType:   eventTypeBitableRecordChangedV1,
			Params:      []event.ParamDef{fileTokenParam("Record changes of other Bases are filtered out.")},
			Schema: event.SchemaDef{
				Native: &event.SchemaSpec{Type: reflect.TypeOf(DriveFileBitableRecordChangedV1Data{})},
				FieldOverrides: map[string]schemas.FieldMeta{
					"/event/operator_id/open_id":  {Kind: "open_id"},
					"/event/operator_id/union_id": {Kind: "union_id"},
					"/event/operator_id/user_id":  {Kind: "user_id"},
				},
			},
			Match:                 bitableFileTokenMatch,
			PreConsume:            bitableSubscriptionPreConsume(eventTypeBitableRecordChangedV1),
			Scopes:                []string{"base:record:read"},
			AuthTypes:             []string{"user", "bot"},
			RequiredConsoleEvents: []string{eventTypeBitableRecordChangedV1},
		},
	}
}
