// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/json"
	"testing"
)

// marshalMeta marshals m and fails the test on error, returning the JSON bytes.
func marshalMeta(t *testing.T, m *Meta) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal(%#v) error = %v", m, err)
	}
	return b
}

// unmarshalMap unmarshals b into a generic map and fails the test on error.
func unmarshalMap(t *testing.T, b []byte) map[string]interface{} {
	t.Helper()
	var got map[string]interface{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", b, err)
	}
	return got
}

func TestMetaNextSerialization_NonEmptyRoundTrips(t *testing.T) {
	m := &Meta{Next: []NextAction{{Label: "poll", Command: "lark-cli agents task get example:x t1"}}}

	got := unmarshalMap(t, marshalMeta(t, m))

	rawNext, ok := got["next"]
	if !ok {
		t.Fatalf("expected \"next\" key, got %#v", got)
	}
	next, ok := rawNext.([]interface{})
	if !ok {
		t.Fatalf("next type = %T, want array", rawNext)
	}
	if len(next) != 1 {
		t.Fatalf("len(next) = %d, want 1", len(next))
	}
	action, ok := next[0].(map[string]interface{})
	if !ok {
		t.Fatalf("next[0] type = %T, want object", next[0])
	}
	if action["label"] != "poll" {
		t.Errorf("next[0].label = %v, want poll", action["label"])
	}
	if action["command"] != "lark-cli agents task get example:x t1" {
		t.Errorf("next[0].command = %v, want the poll command", action["command"])
	}
}

func TestMetaNextSerialization_NilOmitted(t *testing.T) {
	got := unmarshalMap(t, marshalMeta(t, &Meta{Count: 1}))

	if _, ok := got["next"]; ok {
		t.Errorf("nil Next must be omitted, got %#v", got)
	}
	if got["count"] != float64(1) {
		t.Errorf("count = %v, want 1", got["count"])
	}
}

func TestMetaNextSerialization_EmptySliceOmitted(t *testing.T) {
	// A non-nil but empty slice must also be dropped by omitempty (len == 0).
	got := unmarshalMap(t, marshalMeta(t, &Meta{Next: []NextAction{}}))

	if _, ok := got["next"]; ok {
		t.Errorf("empty Next slice must be omitted, got %#v", got)
	}
}

func TestMetaNextSerialization_EmptyFieldsPresent(t *testing.T) {
	// A NextAction with empty fields still serializes: label/command have no
	// omitempty, so they render as empty strings and the entry stays present.
	got := unmarshalMap(t, marshalMeta(t, &Meta{Next: []NextAction{{}}}))

	next, ok := got["next"].([]interface{})
	if !ok || len(next) != 1 {
		t.Fatalf("next = %#v, want single-element array", got["next"])
	}
	action, ok := next[0].(map[string]interface{})
	if !ok {
		t.Fatalf("next[0] type = %T, want object", next[0])
	}
	label, hasLabel := action["label"]
	command, hasCommand := action["command"]
	if !hasLabel || label != "" {
		t.Errorf("label = %v (present=%v), want empty string present", label, hasLabel)
	}
	if !hasCommand || command != "" {
		t.Errorf("command = %v (present=%v), want empty string present", command, hasCommand)
	}
}

func TestMetaNextSerialization_TemplateTruePresent(t *testing.T) {
	// A template hint (command carries <...> placeholders) must serialize the
	// marker so AI callers know it needs substitution before execution.
	m := &Meta{Next: []NextAction{{
		Label:    "continue",
		Command:  "lark-cli agents send example:x --context-id c1 --task-id t1 --text <你的答复>",
		Template: true,
	}}}

	next, ok := unmarshalMap(t, marshalMeta(t, m))["next"].([]interface{})
	if !ok || len(next) != 1 {
		t.Fatalf("next = %#v, want single-element array", next)
	}
	action, _ := next[0].(map[string]interface{})
	if action["template"] != true {
		t.Errorf("template = %v, want true", action["template"])
	}
}

func TestMetaNextSerialization_TemplateFalseOmitted(t *testing.T) {
	// A directly executable hint must not carry the template key at all
	// (omitempty): its absence is the "run verbatim" signal.
	m := &Meta{Next: []NextAction{{Label: "poll", Command: "lark-cli agents task get example:x t1 --watch"}}}

	next, ok := unmarshalMap(t, marshalMeta(t, m))["next"].([]interface{})
	if !ok || len(next) != 1 {
		t.Fatalf("next = %#v, want single-element array", next)
	}
	action, _ := next[0].(map[string]interface{})
	if _, present := action["template"]; present {
		t.Errorf("template=false must be omitted, got %#v", action)
	}
}

func TestMetaNextSerialization_MultipleActionsPreserveOrder(t *testing.T) {
	m := &Meta{Next: []NextAction{
		{Label: "poll", Command: "lark-cli agents task get example:x t1"},
		{Label: "cancel", Command: "lark-cli agents task cancel example:x t1"},
	}}

	next, ok := unmarshalMap(t, marshalMeta(t, m))["next"].([]interface{})
	if !ok || len(next) != 2 {
		t.Fatalf("next = %#v, want two-element array", next)
	}
	first, _ := next[0].(map[string]interface{})
	second, _ := next[1].(map[string]interface{})
	if first["label"] != "poll" || second["label"] != "cancel" {
		t.Errorf("order not preserved: got %v then %v", first["label"], second["label"])
	}
}

func TestMetaNextSerialization_SpecialCharacters(t *testing.T) {
	// Fields carrying quotes, unicode and newlines must survive a JSON round
	// trip intact, which string matching would not reliably verify.
	label := `poll "now"`
	command := "lark-cli agents task get example:代理 t1\n--wait"
	m := &Meta{Next: []NextAction{{Label: label, Command: command}}}

	next, _ := unmarshalMap(t, marshalMeta(t, m))["next"].([]interface{})
	if len(next) != 1 {
		t.Fatalf("next = %#v, want single-element array", next)
	}
	action, _ := next[0].(map[string]interface{})
	if action["label"] != label {
		t.Errorf("label = %q, want %q", action["label"], label)
	}
	if action["command"] != command {
		t.Errorf("command = %q, want %q", action["command"], command)
	}
}

func TestEnvelopeMetaNextIntegration(t *testing.T) {
	// Meta.Next must serialize correctly when nested inside a full Envelope,
	// under the "meta" key alongside data.
	env := Envelope{
		OK:   true,
		Data: map[string]interface{}{"task_id": "t1"},
		Meta: &Meta{Next: []NextAction{{Label: "poll", Command: "lark-cli agents task get example:x t1"}}},
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal(envelope) error = %v", err)
	}
	got := unmarshalMap(t, b)

	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
	meta, ok := got["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("meta type = %T, want object", got["meta"])
	}
	next, ok := meta["next"].([]interface{})
	if !ok || len(next) != 1 {
		t.Fatalf("meta.next = %#v, want single-element array", meta["next"])
	}
	action, _ := next[0].(map[string]interface{})
	if action["command"] != "lark-cli agents task get example:x t1" {
		t.Errorf("meta.next[0].command = %v, want the poll command", action["command"])
	}
}

func TestEnvelopeNilMetaOmitted(t *testing.T) {
	// nil Meta is a valid edge case: the "meta" key must not appear.
	b, err := json.Marshal(Envelope{OK: true})
	if err != nil {
		t.Fatalf("json.Marshal(envelope) error = %v", err)
	}
	got := unmarshalMap(t, b)
	if _, ok := got["meta"]; ok {
		t.Errorf("nil Meta must be omitted, got %#v", got)
	}
}
