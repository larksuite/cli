// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"encoding/json"
	"testing"
)

// OrderedProps 在测试里验证：MarshalJSON 按 Order 切片顺序输出 key，跳过 Go map 默认字母序。
func TestOrderedProps_MarshalJSON_PreservesOrder(t *testing.T) {
	op := &OrderedProps{
		Order: []string{"z_first", "a_second", "m_third"},
		Map: map[string]Property{
			"z_first":  {Type: "string"},
			"a_second": {Type: "integer"},
			"m_third":  {Type: "boolean"},
		},
	}
	b, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	got := string(b)
	want := `{"z_first":{"type":"string"},"a_second":{"type":"integer"},"m_third":{"type":"boolean"}}`
	if got != want {
		t.Errorf("OrderedProps key order not preserved:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestOrderedProps_MarshalJSON_Empty(t *testing.T) {
	op := &OrderedProps{Order: nil, Map: nil}
	b, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if string(b) != "{}" {
		t.Errorf("empty OrderedProps should marshal to {}, got: %s", b)
	}
}

func TestOrderedProps_UnmarshalJSON_RoundTrip(t *testing.T) {
	in := []byte(`{"first":{"type":"string"},"second":{"type":"integer"}}`)
	var op OrderedProps
	if err := json.Unmarshal(in, &op); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(op.Order) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(op.Order))
	}
	if op.Order[0] != "first" || op.Order[1] != "second" {
		t.Errorf("unmarshal lost order: got %v", op.Order)
	}
	if op.Map["first"].Type != "string" {
		t.Errorf("first.type mismatch")
	}
}
