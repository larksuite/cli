// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import (
	"reflect"
	"testing"
)

const sampleJSON = `{
  "version": "1.0.0",
  "services": [
    {
      "name": "im",
      "servicePath": "/open-apis/im/v1",
      "resources": {
        "chat.members": {
          "methods": {
            "create": {
              "httpMethod": "POST",
              "risk": "high-risk-write",
              "parameters": {
                "member_id_type": {"type": "string", "location": "query", "options": [{"value": "open_id"}, {"value": "user_id"}]},
                "chat_id":        {"type": "string", "location": "path", "required": true, "example": "oc_x"},
                "x_header":       {"type": "string", "location": "header"}
              },
              "requestBody": {
                "id_list": {"type": "list", "required": true},
                "avatar":  {"type": "file"}
              }
            }
          }
        }
      }
    }
  ]
}`

func loadSample(t *testing.T) (Resource, Method) {
	t.Helper()
	reg, err := Parse([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	res := reg.Services[0].ResourceList()
	if len(res) != 1 {
		t.Fatalf("want 1 resource, got %d", len(res))
	}
	methods := res[0].MethodList()
	if len(methods) != 1 {
		t.Fatalf("want 1 method, got %d", len(methods))
	}
	return res[0], methods[0]
}

func TestParse_TypedAndNameInjected(t *testing.T) {
	res, m := loadSample(t)
	if res.Name != "chat.members" {
		t.Errorf("resource name = %q, want chat.members", res.Name)
	}
	if m.Name != "create" || m.HTTPMethod != "POST" || m.Risk != "high-risk-write" {
		t.Errorf("method = %+v", m)
	}
}

func TestMethod_AccessorsSortedByName(t *testing.T) {
	_, m := loadSample(t)

	// Params: path/query only (header dropped), sorted by name.
	var params []string
	for _, f := range m.Params() {
		params = append(params, f.Name)
	}
	if want := []string{"chat_id", "member_id_type"}; !reflect.DeepEqual(params, want) {
		t.Errorf("Params() = %v, want %v (sorted, header dropped)", params, want)
	}

	if d := m.Data(); len(d) != 1 || d[0].Name != "id_list" {
		t.Errorf("Data() = %+v, want [id_list]", d)
	}
	if f := m.Files(); len(f) != 1 || f[0].Name != "avatar" {
		t.Errorf("Files() = %+v, want [avatar]", f)
	}
}

func TestField_FlagNameAndOptions(t *testing.T) {
	_, m := loadSample(t)
	by := make(map[string]Field)
	for _, f := range m.Params() {
		by[f.Name] = f
	}

	if got := by["chat_id"].FlagName(); got != "chat-id" {
		t.Errorf("FlagName = %q, want chat-id", got)
	}
	if !by["chat_id"].Required || by["chat_id"].Example != "oc_x" {
		t.Errorf("chat_id required/example wrong: %+v", by["chat_id"])
	}
	opts := by["member_id_type"].Options
	if len(opts) != 2 || opts[0].Value != "open_id" || opts[1].Value != "user_id" {
		t.Errorf("member_id_type options = %+v", opts)
	}
}

// TestParse_TolerantOptionValue guards against whole-catalog blanking: a single
// options[].value that arrives as a JSON number (not the usual quoted string)
// must NOT fail the entire registry unmarshal. Option.Value is `any`, so it
// parses and coerces like Enum instead of returning an empty Registry.
func TestParse_TolerantOptionValue(t *testing.T) {
	data := []byte(`{"services":[{"name":"im","servicePath":"/x","resources":{
		"chat":{"methods":{"create":{"parameters":{
			"flag":{"type":"integer","location":"query","options":[{"value":0,"description":"off"},{"value":1,"description":"on"}]}
		}}}}}}]}`)
	reg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed on numeric option value (would blank the catalog): %v", err)
	}
	if len(reg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d (catalog blanked)", len(reg.Services))
	}
	// The numeric option coerces into the typed enum as sorted int64 (not
	// float64): the integer field's canonical type drives normalization.
	m, _ := reg.Services[0].Resource("chat")
	method, _ := m.Method("create")
	by := map[string]Field{}
	for _, f := range method.Params() {
		by[f.Name] = f
	}
	if got := by["flag"].EnumValues(); !reflect.DeepEqual(got, []any{int64(0), int64(1)}) {
		t.Errorf("numeric-valued enum did not coerce to sorted int64: %#v", got)
	}
}

func TestParse_Empty(t *testing.T) {
	reg, err := Parse(nil)
	if err != nil || len(reg.Services) != 0 {
		t.Fatalf("Parse(nil) = %+v, %v", reg, err)
	}
}
