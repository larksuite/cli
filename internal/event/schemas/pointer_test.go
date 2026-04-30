// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schemas

import (
	"encoding/json"
	"testing"
)

func parseSchema(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return m
}

func TestResolvePointer_SimpleField(t *testing.T) {
	schema := parseSchema(t, `{
        "type":"object",
        "properties":{
            "chat_id":{"type":"string"}
        }
    }`)
	nodes := ResolvePointer(schema, "/chat_id")
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	if nodes[0]["type"] != "string" {
		t.Errorf("got %v", nodes[0])
	}
}

func TestResolvePointer_Nested(t *testing.T) {
	schema := parseSchema(t, `{
        "type":"object",
        "properties":{
            "message":{
                "type":"object",
                "properties":{
                    "chat_type":{"type":"string"}
                }
            }
        }
    }`)
	nodes := ResolvePointer(schema, "/message/chat_type")
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
}

func TestResolvePointer_ArrayElementWildcard(t *testing.T) {
	schema := parseSchema(t, `{
        "type":"object",
        "properties":{
            "message_id_list":{
                "type":"array",
                "items":{"type":"string"}
            }
        }
    }`)
	nodes := ResolvePointer(schema, "/message_id_list/*")
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	if nodes[0]["type"] != "string" {
		t.Errorf("want string items, got %v", nodes[0])
	}
}

func TestResolvePointer_ArrayElementField(t *testing.T) {
	schema := parseSchema(t, `{
        "type":"object",
        "properties":{
            "attachments":{
                "type":"array",
                "items":{
                    "type":"object",
                    "properties":{
                        "mime_type":{"type":"string"}
                    }
                }
            }
        }
    }`)
	nodes := ResolvePointer(schema, "/attachments/*/mime_type")
	if len(nodes) != 1 || nodes[0]["type"] != "string" {
		t.Errorf("want mime_type node, got %v", nodes)
	}
}

func TestResolvePointer_MissingReturnsEmpty(t *testing.T) {
	schema := parseSchema(t, `{"type":"object","properties":{"a":{"type":"string"}}}`)
	nodes := ResolvePointer(schema, "/b/c/d")
	if len(nodes) != 0 {
		t.Errorf("want empty for missing path, got %v", nodes)
	}
}

func TestResolvePointer_RootReturnsSelf(t *testing.T) {
	schema := parseSchema(t, `{"type":"object"}`)
	nodes := ResolvePointer(schema, "")
	if len(nodes) != 1 {
		t.Fatalf("want 1 root node, got %d", len(nodes))
	}
	if nodes[0]["type"] != "object" {
		t.Errorf("root resolution broken")
	}
}
