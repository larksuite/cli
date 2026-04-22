// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schemas

import (
	"encoding/json"
	"reflect"
	"testing"
)

type inner struct {
	OpenID  *string `json:"open_id,omitempty"`
	UserID  *string `json:"user_id,omitempty"`
	UnionID *string `json:"union_id,omitempty"`
}

type sample struct {
	Name          string   `json:"name"`
	Optional      *string  `json:"optional,omitempty"`
	Tags          []string `json:"tags"`
	Reader        *inner   `json:"reader,omitempty"`
	Count         int      `json:"count"`
	Flag          bool     `json:"flag"`
	Skipped       string   `json:"-"`
	unexportedStr string
}

func TestFromType_ScalarAndOptional(t *testing.T) {
	raw := FromType(reflect.TypeOf(sample{}))
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["type"] != "object" {
		t.Errorf("type = %v, want object", parsed["type"])
	}
	props, _ := parsed["properties"].(map[string]interface{})
	if props == nil {
		t.Fatal("properties missing")
	}
	// scalar
	if got := props["name"].(map[string]interface{})["type"]; got != "string" {
		t.Errorf("name.type = %v, want string", got)
	}
	// pointer = optional, same JSON type
	if got := props["optional"].(map[string]interface{})["type"]; got != "string" {
		t.Errorf("optional.type = %v, want string", got)
	}
	// slice → array + items
	tagsNode := props["tags"].(map[string]interface{})
	if tagsNode["type"] != "array" {
		t.Errorf("tags.type = %v, want array", tagsNode["type"])
	}
	if items, ok := tagsNode["items"].(map[string]interface{}); !ok || items["type"] != "string" {
		t.Errorf("tags.items = %v, want string type", tagsNode["items"])
	}
	// nested struct
	readerNode := props["reader"].(map[string]interface{})
	if readerNode["type"] != "object" {
		t.Errorf("reader.type = %v, want object", readerNode["type"])
	}
	// bool / int
	if props["flag"].(map[string]interface{})["type"] != "boolean" {
		t.Errorf("flag.type wrong")
	}
	if props["count"].(map[string]interface{})["type"] != "integer" {
		t.Errorf("count.type wrong")
	}
	// json:"-" skipped
	if _, ok := props["Skipped"]; ok {
		t.Error("Skipped should not be in schema")
	}
	if _, ok := props["-"]; ok {
		t.Error("- should not be in schema")
	}
	// unexported skipped
	if _, ok := props["unexportedStr"]; ok {
		t.Error("unexported field should not be in schema")
	}
}

type descSharedInner struct {
	V string `json:"v"`
}

type descSharedOuter struct {
	Owner  descSharedInner `json:"owner"  desc:"the owner"`
	Member descSharedInner `json:"member" desc:"the member"`
}

// TestFromType_SharedSubtypeDistinctDescriptions guards the shared-cache
// correctness bug introduced by Task 4.3: two fields of the same struct
// type should carry their own `desc` annotation, not a last-write-wins
// value written onto a shared *schemaNode.
func TestFromType_SharedSubtypeDistinctDescriptions(t *testing.T) {
	raw := FromType(reflect.TypeOf(descSharedOuter{}))
	var parsed struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if got := parsed.Properties["owner"].Description; got != "the owner" {
		t.Errorf("owner.description = %q, want %q", got, "the owner")
	}
	if got := parsed.Properties["member"].Description; got != "the member" {
		t.Errorf("member.description = %q, want %q", got, "the member")
	}
}

type mapSample struct {
	Attrs map[string]int `json:"attrs"`
}

// TestFromType_MapAdditionalProperties verifies that a map field emits
// additionalProperties describing the value type — consumers of the
// schema can then tell that Attrs is map[string]int rather than an
// uninstrumented object.
func TestFromType_MapAdditionalProperties(t *testing.T) {
	raw := FromType(reflect.TypeOf(mapSample{}))
	var parsed struct {
		Properties map[string]struct {
			Type                 string `json:"type"`
			AdditionalProperties struct {
				Type string `json:"type"`
			} `json:"additionalProperties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	attrs := parsed.Properties["attrs"]
	if attrs.Type != "object" {
		t.Errorf("attrs.type = %q, want object", attrs.Type)
	}
	if attrs.AdditionalProperties.Type != "integer" {
		t.Errorf("attrs.additionalProperties.type = %q, want integer", attrs.AdditionalProperties.Type)
	}
}

type cyclic struct {
	Name  string   `json:"name"`
	Child *cyclic  `json:"child,omitempty"`
	Kids  []cyclic `json:"kids,omitempty"`
}

func TestFromType_HandlesCycles(t *testing.T) {
	raw := FromType(reflect.TypeOf(cyclic{}))
	if len(raw) == 0 {
		t.Fatal("expected schema for cyclic type")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Just make sure we didn't blow up / stack overflow.
	if parsed["type"] != "object" {
		t.Errorf("cyclic.type = %v", parsed["type"])
	}
}

type embedBase struct {
	EventID string `json:"event_id"`
}

type embedOuter struct {
	*embedBase
	Payload string `json:"payload"`
}

func TestFromType_FlattensEmbeds(t *testing.T) {
	raw := FromType(reflect.TypeOf(embedOuter{}))
	var parsed struct {
		Properties map[string]interface{} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed.Properties["event_id"]; !ok {
		t.Error("embedded field event_id should appear at top level")
	}
	if _, ok := parsed.Properties["payload"]; !ok {
		t.Error("payload should appear")
	}
}

func TestFromType_NilSafe(t *testing.T) {
	if got := FromType(nil); got != nil {
		t.Errorf("FromType(nil) = %s, want nil", got)
	}
}

type tagSample struct {
	ChatType     string `json:"chat_type"     enum:"p2p,group"`
	OpenID       string `json:"open_id"       kind:"open_id"`
	InternalDate string `json:"internal_date" kind:"timestamp_ms"`
	// Array: enum/kind should dive into items, not decorate the array itself.
	Recipients []string `json:"recipients" kind:"email"`
	States     []string `json:"states"     enum:"unread,read,flagged"`
	// Untagged []string sharing the same element reflect.Type as Recipients/States —
	// must NOT inherit items.Format / items.Enum from their tag annotations. Guards
	// the outer-array-node cache clone (without `newArr := *child`, the cache entry
	// for []string would keep whatever items pointer the last tagged field wrote).
	Plain []string `json:"plain"`
}

func TestFromType_EnumAndKindTags(t *testing.T) {
	raw := FromType(reflect.TypeOf(tagSample{}))
	var parsed struct {
		Properties map[string]struct {
			Type   string   `json:"type"`
			Enum   []string `json:"enum"`
			Format string   `json:"format"`
			Items  *struct {
				Type   string   `json:"type"`
				Enum   []string `json:"enum"`
				Format string   `json:"format"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}

	// Scalar: enum applies to the field itself.
	if got := parsed.Properties["chat_type"].Enum; len(got) != 2 || got[0] != "p2p" || got[1] != "group" {
		t.Errorf("chat_type enum = %v, want [p2p group]", got)
	}

	// Scalar: kind → format.
	if got := parsed.Properties["open_id"].Format; got != "open_id" {
		t.Errorf("open_id format = %q, want open_id", got)
	}
	if got := parsed.Properties["internal_date"].Format; got != "timestamp_ms" {
		t.Errorf("internal_date format = %q, want timestamp_ms", got)
	}

	// Array of string: kind should land on items, NOT the array.
	recipients := parsed.Properties["recipients"]
	if recipients.Format != "" {
		t.Errorf("recipients array.format = %q, want empty (format belongs on items)", recipients.Format)
	}
	if recipients.Items == nil || recipients.Items.Format != "email" {
		t.Errorf("recipients.items.format = %q, want email", recipients.Items)
	}

	// Array of string: enum should land on items too.
	states := parsed.Properties["states"]
	if len(states.Enum) != 0 {
		t.Errorf("states array.enum = %v, want empty", states.Enum)
	}
	if states.Items == nil || len(states.Items.Enum) != 3 {
		t.Errorf("states.items.enum = %v, want 3 values", states.Items)
	}

	// Untagged []string: cache for []string must be clean — no format / enum
	// carried over from Recipients / States. Proves the outer-array-node clone.
	plain := parsed.Properties["plain"]
	if plain.Items != nil && (plain.Items.Format != "" || len(plain.Items.Enum) != 0) {
		t.Errorf("plain.items = {format:%q, enum:%v}, want clean (cache must not be polluted)",
			plain.Items.Format, plain.Items.Enum)
	}
}
