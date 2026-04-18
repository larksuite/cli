// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalize_GenericPassthrough(t *testing.T) {
	m := map[string]any{"key": "val"}
	got := normalize(m)
	if !reflect.DeepEqual(got, m) {
		t.Errorf("map passthrough: got %v, want %v", got, m)
	}

	s := "hello"
	if got := normalize(s); got != s {
		t.Errorf("string passthrough: got %v, want %v", got, s)
	}

	if got := normalize(nil); got != nil {
		t.Errorf("nil passthrough: got %v, want nil", got)
	}

	sl := []any{"a", "b"}
	if got := normalize(sl); !reflect.DeepEqual(got, sl) {
		t.Errorf("[]any passthrough: got %v, want %v", got, sl)
	}

	n := json.Number("42")
	if got := normalize(n); got != n {
		t.Errorf("json.Number passthrough: got %v, want %v", got, n)
	}

	b := true
	if got := normalize(b); got != b {
		t.Errorf("bool passthrough: got %v, want %v", got, b)
	}
}

func TestNormalize_TypedStruct(t *testing.T) {
	type S struct{ Msg string }
	got := normalize(S{Msg: "hello"})
	want := map[string]any{"Msg": "hello"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("typed struct: got %v, want %v", got, want)
	}
}

func TestNormalize_TypedStructSlice(t *testing.T) {
	type X struct{ V string }
	input := []*X{{V: "a"}, {V: "b"}}
	got := normalize(input)
	want := []any{
		map[string]any{"V": "a"},
		map[string]any{"V": "b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("typed slice: got %v, want %v", got, want)
	}
}

func TestNormalize_JsonNumberPreserved(t *testing.T) {
	// A typed int64 should round-trip and come back as json.Number
	var x int64 = 9007199254740993
	got := normalize(x)
	n, ok := got.(json.Number)
	if !ok {
		t.Fatalf("expected json.Number, got %T: %v", got, got)
	}
	if n.String() != "9007199254740993" {
		t.Errorf("number value: got %q, want %q", n.String(), "9007199254740993")
	}
}

func TestNormalize_Unmarshalable(t *testing.T) {
	// A struct with a func field cannot be marshaled; normalize must return original.
	type Bad struct{ F func() }
	v := Bad{F: func() {}}
	got := normalize(v)
	// Just verify no panic and that we got something back (the original value).
	if got == nil {
		t.Error("expected non-nil return for unmarshalable value")
	}
}
