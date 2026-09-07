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

// syncToChatOutputKeys is the full set of output keys the relation can produce;
// "omitted" assertions must cover all of them, not just one.
var syncToChatOutputKeys = []string{"synced_from_thread_reply", "synced_from_thread", "synced_to_chat_message"}

func TestSyncToChatRelationAccessors(t *testing.T) {
	tests := []struct {
		name                string
		relation            *SyncToChatRelation
		wantFromThreadReply string
		wantFromThread      string
		wantToChatMessage   string
	}{
		{
			name:                "chat copy exposes the reply it came from",
			relation:            &SyncToChatRelation{Type: 1, ThreadID: "omt_root", RelatedMessageID: "om_reply"},
			wantFromThreadReply: "om_reply",
			wantFromThread:      "omt_root",
		},
		{
			name:                "chat copy without thread_id still exposes the reply",
			relation:            &SyncToChatRelation{Type: 1, RelatedMessageID: "om_reply"},
			wantFromThreadReply: "om_reply",
		},
		{
			name:              "thread reply exposes its chat copy",
			relation:          &SyncToChatRelation{Type: 2, RelatedMessageID: "om_copy"},
			wantToChatMessage: "om_copy",
		},
		{
			name: "thread reply never leaks thread_id as synced_from_thread",
			// Upstream sets thread_id only on the copy, but be explicit: a
			// type-2 relation must not populate any synced_from_* field.
			relation:          &SyncToChatRelation{Type: 2, ThreadID: "omt_root", RelatedMessageID: "om_copy"},
			wantToChatMessage: "om_copy",
		},
		{name: "nil relation yields nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.relation.SyncedFromThreadReply(); got != tt.wantFromThreadReply {
				t.Errorf("SyncedFromThreadReply() = %q, want %q", got, tt.wantFromThreadReply)
			}
			if got := tt.relation.SyncedFromThread(); got != tt.wantFromThread {
				t.Errorf("SyncedFromThread() = %q, want %q", got, tt.wantFromThread)
			}
			if got := tt.relation.SyncedToChatMessage(); got != tt.wantToChatMessage {
				t.Errorf("SyncedToChatMessage() = %q, want %q", got, tt.wantToChatMessage)
			}
		})
	}
}

func TestApplySyncToChatFields(t *testing.T) {
	tests := []struct {
		name     string
		relation *SyncToChatRelation
		want     map[string]interface{}
	}{
		{
			name:     "chat copy",
			relation: &SyncToChatRelation{Type: 1, ThreadID: "omt_root", RelatedMessageID: "om_reply"},
			want:     map[string]interface{}{"synced_from_thread_reply": "om_reply", "synced_from_thread": "omt_root"},
		},
		{
			name:     "chat copy without thread_id omits synced_from_thread",
			relation: &SyncToChatRelation{Type: 1, RelatedMessageID: "om_reply"},
			want:     map[string]interface{}{"synced_from_thread_reply": "om_reply"},
		},
		{
			name:     "thread reply",
			relation: &SyncToChatRelation{Type: 2, RelatedMessageID: "om_copy"},
			want:     map[string]interface{}{"synced_to_chat_message": "om_copy"},
		},
		{name: "nil relation writes nothing", want: map[string]interface{}{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := map[string]interface{}{}
			ApplySyncToChatFields(got, tt.relation)
			if len(got) != len(tt.want) {
				t.Fatalf("ApplySyncToChatFields() = %#v, want %#v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("ApplySyncToChatFields()[%s] = %#v, want %#v", k, got[k], v)
				}
			}
		})
	}
}

func TestApplySyncToChatFieldsNilMap(t *testing.T) {
	// Must not panic; there is no map to write into.
	ApplySyncToChatFields(nil, &SyncToChatRelation{Type: 1, RelatedMessageID: "om_reply"})
}

func TestDecodeSyncToChatRelationTrimsIDs(t *testing.T) {
	got := DecodeSyncToChatRelation([]byte(`{"type":1,"thread_id":" omt_root ","related_message_id":" om_reply "}`))
	if got == nil {
		t.Fatal("DecodeSyncToChatRelation() = nil, want relation")
	}
	if got.RelatedMessageID != "om_reply" || got.ThreadID != "omt_root" {
		t.Fatalf("DecodeSyncToChatRelation() = %#v, want trimmed IDs", got)
	}
}
