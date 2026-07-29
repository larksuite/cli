// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package application registers Application-domain EventKeys.
package application

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const eventTypeBotMenuV6 = "application.bot.menu_v6"

// Keys returns all Application-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         eventTypeBotMenuV6,
			DisplayName: "Bot menu",
			Description: "Triggered when a user clicks a custom bot menu item whose action is configured as a push event.",
			EventType:   eventTypeBotMenuV6,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(BotMenuOutput{})},
			},
			Process:               processBotMenu,
			AuthTypes:             []string{"bot"},
			RequiredConsoleEvents: []string{eventTypeBotMenuV6},
		},
	}
}
