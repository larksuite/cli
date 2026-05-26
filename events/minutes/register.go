// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package minutes registers Minutes-domain EventKeys.
package minutes

import (
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const (
	eventTypeMinuteGenerated = "minutes.minute.generated_v1"

	pathMinuteSubscribe   = "/open-apis/minutes/v1/minutes/subscription"
	pathMinuteUnsubscribe = "/open-apis/minutes/v1/minutes/unsubscription"

	pathMinuteDetailFmt = "/open-apis/minutes/v1/minutes/%s"
)

// Keys returns all Minutes-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         eventTypeMinuteGenerated,
			DisplayName: "Minute generated",
			Description: "Triggered when a minute has been generated",
			EventType:   eventTypeMinuteGenerated,
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(MinutesMinuteGeneratedOutput{})},
			},
			Process:    processMinutesMinuteGenerated,
			PreConsume: subscriptionPreConsume(eventTypeMinuteGenerated, pathMinuteSubscribe, pathMinuteUnsubscribe),
			Scopes:     []string{"minutes:minutes.basic:read"},
			AuthTypes: []string{
				"user",
			},
			RequiredConsoleEvents: []string{eventTypeMinuteGenerated},
		},
	}
}
