// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package task registers Task-domain EventKeys.
package task

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const eventTypeTaskUpdateUserAccessV2 = "task.task.update_user_access_v2"

// Keys returns all Task-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         eventTypeTaskUpdateUserAccessV2,
			DisplayName: "Task updated",
			Description: "Triggered when tasks visible to the current user or app are created, deleted, or updated",
			EventType:   eventTypeTaskUpdateUserAccessV2,
			Schema: event.SchemaDef{
				Native: &event.SchemaSpec{Type: reflect.TypeOf(TaskUpdateUserAccessV2Data{})},
			},
			PreConsume:            taskSubscriptionPreConsume,
			Scopes:                []string{"task:task:read"},
			AuthTypes:             []string{"user", "bot"},
			RequiredConsoleEvents: []string{eventTypeTaskUpdateUserAccessV2},
			SingleConsumer:        true,
		},
	}
}
