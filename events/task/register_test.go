// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/schemas"
)

func TestKeysTaskUpdateUserAccessMetadata(t *testing.T) {
	keys := Keys()
	if len(keys) != 1 {
		t.Fatalf("len(Keys()) = %d, want 1", len(keys))
	}

	def := keys[0]
	if def.Key != eventTypeTaskUpdateUserAccessV2 {
		t.Errorf("Key = %q, want %q", def.Key, eventTypeTaskUpdateUserAccessV2)
	}
	if def.EventType != eventTypeTaskUpdateUserAccessV2 {
		t.Errorf("EventType = %q, want %q", def.EventType, eventTypeTaskUpdateUserAccessV2)
	}
	if def.Schema.Native == nil {
		t.Fatal("Schema.Native is nil")
	}
	if def.Schema.Native.Type != reflect.TypeOf(TaskUpdateUserAccessV2Data{}) {
		t.Errorf("native type = %v, want TaskUpdateUserAccessV2Data", def.Schema.Native.Type)
	}
	if def.Process != nil {
		t.Fatal("Native Task EventKey must not set Process")
	}
	if def.PreConsume == nil {
		t.Fatal("PreConsume is nil")
	}
	if !def.SingleConsumer {
		t.Fatal("SingleConsumer = false, want true")
	}
	if !reflect.DeepEqual(def.Scopes, []string{"task:task:read"}) {
		t.Errorf("Scopes = %#v", def.Scopes)
	}
	if !reflect.DeepEqual(def.AuthTypes, []string{"user", "bot"}) {
		t.Errorf("AuthTypes = %#v", def.AuthTypes)
	}
	if !reflect.DeepEqual(def.RequiredConsoleEvents, []string{eventTypeTaskUpdateUserAccessV2}) {
		t.Errorf("RequiredConsoleEvents = %#v", def.RequiredConsoleEvents)
	}
}

func TestTaskUpdateUserAccessSchemaAnnotations(t *testing.T) {
	raw := schemas.WrapV2Envelope(schemas.FromType(reflect.TypeOf(TaskUpdateUserAccessV2Data{})))
	var schema map[string]interface{}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	eventProps := schema["properties"].(map[string]interface{})["event"].(map[string]interface{})["properties"].(map[string]interface{})
	taskGUID := eventProps["task_guid"].(map[string]interface{})
	if got := taskGUID["format"]; got != "task_guid" {
		t.Errorf("task_guid format = %v, want task_guid", got)
	}

	eventTypes := eventProps["event_types"].(map[string]interface{})
	items := eventTypes["items"].(map[string]interface{})
	rawEnum, ok := items["enum"].([]interface{})
	if !ok {
		t.Fatalf("event_types item enum missing: %#v", items["enum"])
	}
	got := make(map[string]bool, len(rawEnum))
	for _, v := range rawEnum {
		got[v.(string)] = true
	}
	for _, want := range taskUpdateUserAccessCommitTypes {
		if !got[want] {
			t.Errorf("event_types enum missing %q; enum=%v", want, rawEnum)
		}
	}
}

func TestTaskUpdateUserAccessRegistersCleanly(t *testing.T) {
	const key = eventTypeTaskUpdateUserAccessV2
	event.UnregisterKeyForTest(key)
	t.Cleanup(func() { event.UnregisterKeyForTest(key) })

	for _, def := range Keys() {
		event.RegisterKey(def)
	}
	if _, ok := event.Lookup(key); !ok {
		t.Fatalf("event.Lookup(%q) not registered", key)
	}
}
