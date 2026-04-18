// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"testing"
)

func TestReadJSONPointer_EmptyPointer(t *testing.T) {
	data := map[string]interface{}{"key": "value"}
	got, err := ReadJSONPointer(data, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if m["key"] != "value" {
		t.Errorf("got %v, want map with key=value", m)
	}
}

func TestReadJSONPointer_OneLevel(t *testing.T) {
	data := map[string]interface{}{"key": "hello"}
	got, err := ReadJSONPointer(data, "/key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %v, want %q", got, "hello")
	}
}

func TestReadJSONPointer_TwoLevels(t *testing.T) {
	data := map[string]interface{}{
		"key": map[string]interface{}{
			"subkey": "deep_value",
		},
	}
	got, err := ReadJSONPointer(data, "/key/subkey")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "deep_value" {
		t.Errorf("got %v, want %q", got, "deep_value")
	}
}

func TestReadJSONPointer_MissingKey(t *testing.T) {
	data := map[string]interface{}{"key": "value"}
	_, err := ReadJSONPointer(data, "/nonexistent")
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
	want := `json pointer "/nonexistent": key "nonexistent" not found`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestReadJSONPointer_NonMapIntermediate(t *testing.T) {
	data := map[string]interface{}{"key": "scalar_string"}
	_, err := ReadJSONPointer(data, "/key/subkey")
	if err == nil {
		t.Fatal("expected error for non-map intermediate, got nil")
	}
	want := `json pointer "/key/subkey": value at "/key" is string, not an object`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestReadJSONPointer_RFC6901_Escaping(t *testing.T) {
	// ~1 decodes to / and ~0 decodes to ~
	data := map[string]interface{}{
		"a/b": "slash_value",
		"c~d": "tilde_value",
	}

	// ~1 -> /
	got, err := ReadJSONPointer(data, "/a~1b")
	if err != nil {
		t.Fatalf("unexpected error for ~1 escape: %v", err)
	}
	if got != "slash_value" {
		t.Errorf("got %v, want %q", got, "slash_value")
	}

	// ~0 -> ~
	got, err = ReadJSONPointer(data, "/c~0d")
	if err != nil {
		t.Fatalf("unexpected error for ~0 escape: %v", err)
	}
	if got != "tilde_value" {
		t.Errorf("got %v, want %q", got, "tilde_value")
	}
}

func TestReadJSONPointer_InvalidFormat(t *testing.T) {
	data := map[string]interface{}{"key": "val"}
	_, err := ReadJSONPointer(data, "no-leading-slash")
	if err == nil {
		t.Fatal("expected error for pointer without leading /")
	}
	want := `json pointer must start with '/' or be empty, got "no-leading-slash"`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
