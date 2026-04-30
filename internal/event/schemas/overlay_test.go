// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schemas

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestApplyFieldOverrides_AddDescriptionEnumFormat(t *testing.T) {
	schema := parseSchema(t, `{
        "type":"object",
        "properties":{
            "message_type":{"type":"string"},
            "sender_id":{"type":"string"}
        }
    }`)
	overrides := map[string]FieldMeta{
		"/message_type": {Enum: []string{"text", "post"}, Description: "消息类型"},
		"/sender_id":    {Kind: "open_id"},
	}
	orphans := ApplyFieldOverrides(schema, overrides)
	if len(orphans) != 0 {
		t.Fatalf("unexpected orphans: %v", orphans)
	}

	msgType := schema["properties"].(map[string]interface{})["message_type"].(map[string]interface{})
	if msgType["description"] != "消息类型" {
		t.Errorf("description not applied: %v", msgType)
	}
	if enumRaw, ok := msgType["enum"].([]interface{}); !ok || len(enumRaw) != 2 {
		t.Errorf("enum not applied: %v", msgType)
	}

	senderID := schema["properties"].(map[string]interface{})["sender_id"].(map[string]interface{})
	if senderID["format"] != "open_id" {
		t.Errorf("format not applied: %v", senderID)
	}
}

func TestApplyFieldOverrides_OverridesStructTagValues(t *testing.T) {
	schema := parseSchema(t, `{
        "type":"object",
        "properties":{
            "chat_type":{"type":"string","description":"from tag","enum":["old_a","old_b"]}
        }
    }`)
	overrides := map[string]FieldMeta{
		"/chat_type": {Description: "from overlay", Enum: []string{"new_a"}},
	}
	ApplyFieldOverrides(schema, overrides)
	ct := schema["properties"].(map[string]interface{})["chat_type"].(map[string]interface{})
	if ct["description"] != "from overlay" {
		t.Errorf("overlay description should override tag: %v", ct)
	}
	enumRaw := ct["enum"].([]interface{})
	if len(enumRaw) != 1 || enumRaw[0] != "new_a" {
		t.Errorf("overlay enum should override tag: %v", ct)
	}
}

func TestApplyFieldOverrides_OrphanPathsReported(t *testing.T) {
	schema := parseSchema(t, `{"type":"object","properties":{"a":{"type":"string"}}}`)
	overrides := map[string]FieldMeta{
		"/a":     {Kind: "open_id"},
		"/x/y":   {Kind: "chat_id"},
		"/a/bad": {Kind: "chat_id"},
	}
	orphans := ApplyFieldOverrides(schema, overrides)
	want := []string{"/a/bad", "/x/y"}
	if !reflect.DeepEqual(orphans, want) {
		t.Errorf("orphans = %v, want %v", orphans, want)
	}
}

func TestApplyFieldOverrides_ArrayItemsWildcard(t *testing.T) {
	schema := parseSchema(t, `{
        "type":"object",
        "properties":{
            "ids":{"type":"array","items":{"type":"string"}}
        }
    }`)
	overrides := map[string]FieldMeta{
		"/ids/*": {Kind: "message_id"},
	}
	if orphans := ApplyFieldOverrides(schema, overrides); len(orphans) != 0 {
		t.Fatalf("unexpected orphans: %v", orphans)
	}
	items := schema["properties"].(map[string]interface{})["ids"].(map[string]interface{})["items"].(map[string]interface{})
	if items["format"] != "message_id" {
		t.Errorf("items.format not applied: %v", items)
	}
}

type overlaySample struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
}

func TestApplyFieldOverrides_OnReflectedSchema(t *testing.T) {
	raw := FromType(reflect.TypeOf(overlaySample{}))
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	ApplyFieldOverrides(m, map[string]FieldMeta{
		"/message_id": {Kind: "message_id"},
	})
	props := m["properties"].(map[string]interface{})
	mid := props["message_id"].(map[string]interface{})
	if mid["format"] != "message_id" {
		t.Errorf("format not applied to reflected schema: %v", mid)
	}
}
