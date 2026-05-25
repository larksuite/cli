// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/event"
)

func TestMatchMailbox(t *testing.T) {
	makeV2Envelope := func(mailAddr string) json.RawMessage {
		return json.RawMessage(`{"schema":"2.0","header":{},"event":{"mail_address":"` + mailAddr + `"}}`)
	}
	tests := []struct {
		name    string
		payload json.RawMessage
		params  map[string]string
		want    bool
	}{
		{
			name:    "exact match",
			payload: makeV2Envelope("alice@example.com"),
			params:  map[string]string{"mailbox": "alice@example.com"},
			want:    true,
		},
		{
			name:    "mismatch drops",
			payload: makeV2Envelope("bob@example.com"),
			params:  map[string]string{"mailbox": "alice@example.com"},
			want:    false,
		},
		{
			name:    "empty params accepts all (fail-open: no filter)",
			payload: makeV2Envelope("anything@example.com"),
			params:  map[string]string{},
			want:    true,
		},
		{
			name:    "malformed payload fail-open",
			payload: json.RawMessage(`not json`),
			params:  map[string]string{"mailbox": "alice@example.com"},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := &event.RawEvent{Payload: tt.payload}
			if got := matchMailbox(raw, tt.params); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// Compile-time: matchMailbox must match the Match signature.
var _ func(*event.RawEvent, map[string]string) bool = matchMailbox
