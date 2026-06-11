// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/event"
)

func TestComputeSubscriptionID(t *testing.T) {
	makeDef := func(subKeyNames ...string) *event.KeyDefinition {
		def := &event.KeyDefinition{Key: "test.evt"}
		marked := make(map[string]bool, len(subKeyNames))
		for _, n := range subKeyNames {
			marked[n] = true
		}
		for _, n := range []string{"alpha", "beta", "gamma"} {
			def.Params = append(def.Params, event.ParamDef{Name: n, SubscriptionKey: marked[n]})
		}
		return def
	}

	t.Run("no SubscriptionKey params returns EventKey verbatim", func(t *testing.T) {
		def := makeDef()
		got := ComputeSubscriptionID(def, map[string]string{"alpha": "x", "beta": "y"})
		if got != "test.evt" {
			t.Errorf("got %q, want %q", got, "test.evt")
		}
	})

	t.Run("single SubscriptionKey param: non-sub params do not leak into ID", func(t *testing.T) {
		def := makeDef("alpha")
		id1 := ComputeSubscriptionID(def, map[string]string{"alpha": "value1", "beta": "ignored"})
		id2 := ComputeSubscriptionID(def, map[string]string{"alpha": "value1", "beta": "different"})
		if id1 != id2 {
			t.Errorf("non-SubscriptionKey param change leaked into ID: %q vs %q", id1, id2)
		}
	})

	t.Run("different SubscriptionKey value produces different ID", func(t *testing.T) {
		def := makeDef("alpha")
		id1 := ComputeSubscriptionID(def, map[string]string{"alpha": "v1"})
		id2 := ComputeSubscriptionID(def, map[string]string{"alpha": "v2"})
		if id1 == id2 {
			t.Errorf("different values produced same ID: %q", id1)
		}
	})
}

func TestComputeSubscriptionID_Stability(t *testing.T) {
	// Param order in the ParamDef list must not affect the result (sorted by name internally).
	def1 := &event.KeyDefinition{
		Key: "test.evt",
		Params: []event.ParamDef{
			{Name: "b", SubscriptionKey: true},
			{Name: "a", SubscriptionKey: true},
		},
	}
	def2 := &event.KeyDefinition{
		Key: "test.evt",
		Params: []event.ParamDef{
			{Name: "a", SubscriptionKey: true},
			{Name: "b", SubscriptionKey: true},
		},
	}
	id1 := ComputeSubscriptionID(def1, map[string]string{"a": "1", "b": "2"})
	id2 := ComputeSubscriptionID(def2, map[string]string{"a": "1", "b": "2"})
	if id1 != id2 {
		t.Errorf("order-sensitive: id1=%q id2=%q", id1, id2)
	}
}

func TestComputeSubscriptionID_Format(t *testing.T) {
	def := &event.KeyDefinition{
		Key:    "mail.user_mailbox.event.message_received_v1",
		Params: []event.ParamDef{{Name: "mailbox", SubscriptionKey: true}},
	}
	id := ComputeSubscriptionID(def, map[string]string{"mailbox": "liuxinyang@example.com"})
	prefix := "mail.user_mailbox.event.message_received_v1:"
	if !strings.HasPrefix(id, prefix) {
		t.Fatalf("missing prefix: %q", id)
	}
	suffix := strings.TrimPrefix(id, prefix)
	if len(suffix) != 16 {
		t.Errorf("fingerprint length = %d, want 16", len(suffix))
	}
	for _, c := range suffix {
		isValid := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if !isValid {
			t.Errorf("non-base64URL char in fingerprint: %q", suffix)
			break
		}
	}
}

func TestComputeSubscriptionID_UnicodeAndSpecialChars(t *testing.T) {
	def := &event.KeyDefinition{
		Key:    "test.evt",
		Params: []event.ParamDef{{Name: "value", SubscriptionKey: true}},
	}
	for _, val := range []string{"中文", "emoji🚀", "with spaces", "with:colons", "with\"quotes"} {
		id := ComputeSubscriptionID(def, map[string]string{"value": val})
		if !strings.HasPrefix(id, "test.evt:") || len(id) != len("test.evt:")+16 {
			t.Errorf("ID malformed for value=%q: %q (len=%d)", val, id, len(id))
		}
	}
}

func TestComputeSubscriptionID_EmptyValue(t *testing.T) {
	def := &event.KeyDefinition{
		Key:    "test.evt",
		Params: []event.ParamDef{{Name: "x", SubscriptionKey: true}},
	}
	id1 := ComputeSubscriptionID(def, map[string]string{"x": ""})
	id2 := ComputeSubscriptionID(def, map[string]string{}) // missing entirely
	if id1 != id2 {
		t.Errorf("empty value should be indistinguishable from missing: %q vs %q", id1, id2)
	}
	id3 := ComputeSubscriptionID(def, map[string]string{"x": "nonempty"})
	if id1 == id3 {
		t.Errorf("empty and nonempty produced same ID: %q", id1)
	}
}
