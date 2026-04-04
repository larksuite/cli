// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestPrintJson_InjectNotice_Map(t *testing.T) {
	origNotice := PendingNotice
	PendingNotice = func() map[string]interface{} {
		return map[string]interface{}{"update": "available"}
	}
	defer func() { PendingNotice = origNotice }()

	data := map[string]interface{}{"ok": true, "data": "test"}
	var buf bytes.Buffer
	PrintJson(&buf, data)

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	notice, ok := got["_notice"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _notice in map-based envelope")
	}
	if notice["update"] != "available" {
		t.Errorf("expected update=available, got %v", notice["update"])
	}
}

func TestPrintJson_InjectNotice_SkipsNonEnvelope(t *testing.T) {
	origNotice := PendingNotice
	PendingNotice = func() map[string]interface{} {
		return map[string]interface{}{"update": "available"}
	}
	defer func() { PendingNotice = origNotice }()

	// Map without "ok" key should not get _notice
	data := map[string]interface{}{"name": "test"}
	var buf bytes.Buffer
	PrintJson(&buf, data)

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if _, ok := got["_notice"]; ok {
		t.Error("expected no _notice for non-envelope map")
	}
}

func TestPrintJson_Struct_PreservesNotice(t *testing.T) {
	origNotice := PendingNotice
	PendingNotice = nil // no global notice
	defer func() { PendingNotice = origNotice }()

	// Struct with Notice already set should preserve it
	env := &Envelope{
		OK:       true,
		Identity: "user",
		Data:     "hello",
		Notice:   map[string]interface{}{"update": "set-by-caller"},
	}
	var buf bytes.Buffer
	PrintJson(&buf, env)

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	notice, ok := got["_notice"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _notice from struct field")
	}
	if notice["update"] != "set-by-caller" {
		t.Errorf("expected update=set-by-caller, got %v", notice["update"])
	}
}

func TestPrintJson_NoNotice(t *testing.T) {
	origNotice := PendingNotice
	PendingNotice = nil
	defer func() { PendingNotice = origNotice }()

	data := map[string]interface{}{"ok": true, "data": "test"}
	var buf bytes.Buffer
	PrintJson(&buf, data)

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if _, ok := got["_notice"]; ok {
		t.Error("expected no _notice when PendingNotice is nil")
	}
}
