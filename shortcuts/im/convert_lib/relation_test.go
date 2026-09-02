// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"testing"
)

func TestDecodeSyncToChatRelation(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want *SyncToChatRelation
	}{
		{name: "target message relation", raw: `{"type":1,"thread_id":"omt_root","related_message_id":"om_source"}`, want: &SyncToChatRelation{Type: 1, ThreadID: "omt_root", RelatedMessageID: "om_source"}},
		{name: "source message relation", raw: `{"type":2,"related_message_id":"om_target"}`, want: &SyncToChatRelation{Type: 2, RelatedMessageID: "om_target"}},
		{name: "null", raw: `null`},
		{name: "empty object", raw: `{}`},
		{name: "unsupported type", raw: `{"type":3,"related_message_id":"om_other"}`},
		{name: "missing related message", raw: `{"type":1}`},
		{name: "blank related message", raw: `{"type":1,"related_message_id":"  "}`},
		{name: "wrong type field", raw: `{"type":"1","related_message_id":"om_other"}`},
		{name: "wrong related message field", raw: `{"type":1,"related_message_id":42}`},
		{name: "malformed json", raw: `{`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeSyncToChatRelation([]byte(tt.raw))
			if tt.want == nil {
				if got != nil {
					t.Fatalf("DecodeSyncToChatRelation(%s) = %#v, want nil", tt.raw, got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("DecodeSyncToChatRelation(%s) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDecodeSyncToChatRelationIgnoresUnknownFields(t *testing.T) {
	relation := DecodeSyncToChatRelation([]byte(`{"type":1,"thread_id":"omt_root","related_message_id":"om_source","future_field":"private"}`))
	if relation == nil {
		t.Fatal("DecodeSyncToChatRelation() = nil, want relation")
	}
	raw, err := json.Marshal(relation)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := fields["future_field"]; ok {
		t.Fatalf("unknown field leaked into public output: %s", raw)
	}
}

func TestProjectSyncToChatRelation(t *testing.T) {
	got := ProjectSyncToChatRelation(map[string]interface{}{
		"type":               float64(2),
		"related_message_id": "om_target",
		"future_field":       "ignored",
	})
	if got == nil || got.Type != 2 || got.RelatedMessageID != "om_target" {
		t.Fatalf("ProjectSyncToChatRelation() = %#v", got)
	}
	if got := ProjectSyncToChatRelation(map[string]interface{}{}); got != nil {
		t.Fatalf("ProjectSyncToChatRelation(empty) = %#v, want nil", got)
	}
}
