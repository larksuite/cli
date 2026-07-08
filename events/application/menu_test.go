// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

func TestKeysBotMenuMetadata(t *testing.T) {
	keys := Keys()
	if len(keys) != 1 {
		t.Fatalf("len(Keys()) = %d, want 1", len(keys))
	}

	def := keys[0]
	if def.Key != eventTypeBotMenuV6 {
		t.Errorf("Key = %q, want %q", def.Key, eventTypeBotMenuV6)
	}
	if def.EventType != eventTypeBotMenuV6 {
		t.Errorf("EventType = %q, want %q", def.EventType, eventTypeBotMenuV6)
	}
	if def.SubscriptionType != "" {
		t.Errorf("SubscriptionType = %q, want default event subscription", def.SubscriptionType)
	}
	if def.Schema.Custom == nil {
		t.Fatal("Schema.Custom is nil")
	}
	if def.Schema.Custom.Type != reflect.TypeOf(BotMenuOutput{}) {
		t.Errorf("custom type = %v, want BotMenuOutput", def.Schema.Custom.Type)
	}
	if def.Schema.Native != nil {
		t.Fatal("Schema.Native must be nil for processed output")
	}
	if def.Process == nil {
		t.Fatal("Process is nil")
	}
	if !reflect.DeepEqual(def.AuthTypes, []string{"bot"}) {
		t.Errorf("AuthTypes = %#v", def.AuthTypes)
	}
	if !reflect.DeepEqual(def.RequiredConsoleEvents, []string{eventTypeBotMenuV6}) {
		t.Errorf("RequiredConsoleEvents = %#v", def.RequiredConsoleEvents)
	}
}

func TestBotMenuRegistersCleanly(t *testing.T) {
	const key = eventTypeBotMenuV6
	event.UnregisterKeyForTest(key)
	t.Cleanup(func() { event.UnregisterKeyForTest(key) })

	for _, def := range Keys() {
		event.RegisterKey(def)
	}
	if _, ok := event.Lookup(key); !ok {
		t.Fatalf("event.Lookup(%q) not registered", key)
	}
}

func TestProcessBotMenu(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_menu_001",
			"event_type": "application.bot.menu_v6",
			"create_time": "1776409469273",
			"app_id": "cli_test",
			"tenant_key": "tenant_test"
		},
		"event": {
			"event_key": "start_eval",
			"timestamp": 1776409469000,
			"operator": {
				"operator_id": {
					"open_id": "ou_operator",
					"union_id": "on_operator",
					"user_id": "user_operator"
				},
				"operator_name": "Test User"
			}
		}
	}`
	out := runBotMenu(t, payload)

	if out.Type != eventTypeBotMenuV6 {
		t.Errorf("Type = %q, want %q", out.Type, eventTypeBotMenuV6)
	}
	if out.EventID != "ev_menu_001" {
		t.Errorf("EventID = %q", out.EventID)
	}
	if out.Timestamp != "1776409469273" {
		t.Errorf("Timestamp = %q", out.Timestamp)
	}
	if out.EventKey != "start_eval" {
		t.Errorf("EventKey = %q", out.EventKey)
	}
	if out.MenuTimestamp != "1776409469000" {
		t.Errorf("MenuTimestamp = %q", out.MenuTimestamp)
	}
	if out.OperatorID != "ou_operator" || out.OperatorOpenID != "ou_operator" {
		t.Errorf("OperatorID/OperatorOpenID = %q/%q", out.OperatorID, out.OperatorOpenID)
	}
	if out.OperatorUnionID != "on_operator" {
		t.Errorf("OperatorUnionID = %q", out.OperatorUnionID)
	}
	if out.OperatorUserID != "user_operator" {
		t.Errorf("OperatorUserID = %q", out.OperatorUserID)
	}
	if out.OperatorName != "Test User" {
		t.Errorf("OperatorName = %q", out.OperatorName)
	}
	if out.AppID != "cli_test" || out.TenantKey != "tenant_test" {
		t.Errorf("AppID/TenantKey = %q/%q", out.AppID, out.TenantKey)
	}
}

func TestProcessBotMenuStringTimestampFallback(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_menu_002",
			"event_type": "application.bot.menu_v6"
		},
		"event": {
			"event_key": "start_eval",
			"timestamp": "1776409469001",
			"operator": {
				"operator_id": {"open_id": "ou_operator"}
			}
		}
	}`
	out := runBotMenu(t, payload)

	if out.Timestamp != "1776409469001" {
		t.Errorf("Timestamp fallback = %q", out.Timestamp)
	}
	if out.MenuTimestamp != "1776409469001" {
		t.Errorf("MenuTimestamp = %q", out.MenuTimestamp)
	}
}

func TestProcessBotMenuSecondsTimestampFallback(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_menu_seconds",
			"event_type": "application.bot.menu_v6"
		},
		"event": {
			"event_key": "start_eval",
			"timestamp": 1694592375,
			"operator": {
				"operator_id": {"open_id": "ou_operator"}
			}
		}
	}`
	out := runBotMenu(t, payload)

	if out.Timestamp != "1694592375000" {
		t.Errorf("Timestamp fallback = %q, want seconds normalized to milliseconds", out.Timestamp)
	}
	if out.MenuTimestamp != "1694592375000" {
		t.Errorf("MenuTimestamp = %q, want seconds normalized to milliseconds", out.MenuTimestamp)
	}
}

func TestProcessBotMenuTypeUsesLocalConstant(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_menu_003",
			"event_type": "unexpected.event_type",
			"create_time": "1776409469275"
		},
		"event": {
			"event_key": "start_eval",
			"operator": {
				"operator_id": {"open_id": "ou_operator"}
			}
		}
	}`
	out := runBotMenu(t, payload)

	if out.Type != eventTypeBotMenuV6 {
		t.Errorf("Type = %q, want %q", out.Type, eventTypeBotMenuV6)
	}
}

func TestProcessBotMenuMalformedPayload(t *testing.T) {
	raw := &event.RawEvent{
		EventID:   "ev_bad",
		EventType: eventTypeBotMenuV6,
		Payload:   json.RawMessage(`not json`),
		Timestamp: time.Now(),
	}
	got, err := processBotMenu(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process should swallow parse errors, got %v", err)
	}
	if string(got) != "not json" {
		t.Errorf("malformed fallback output = %q, want original bytes", string(got))
	}
}

func runBotMenu(t *testing.T, payload string) BotMenuOutput {
	t.Helper()
	raw := &event.RawEvent{
		EventID:   "ev_test",
		EventType: eventTypeBotMenuV6,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := processBotMenu(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("processBotMenu: %v", err)
	}
	var out BotMenuOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, got)
	}
	return out
}
