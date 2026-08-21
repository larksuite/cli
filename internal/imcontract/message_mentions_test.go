// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestBuildMessageMentionResult(t *testing.T) {
	tests := []struct {
		name          string
		request       MessageMentionRequest
		response      any
		wantStatus    string
		wantConfirmed int
		wantMissing   []string
		wantUnattrib  []string
		wantAll       string
	}{
		{
			name:       "all accepted without notification proof",
			request:    MessageMentionRequest{All: true},
			wantStatus: "accepted_unverified",
			wantAll:    "accepted_unverified",
		},
		{
			name:    "all ignores unverified response shape",
			request: MessageMentionRequest{All: true},
			response: []any{
				map[string]any{"key": "@_all", "id": "all"},
			},
			wantStatus: "accepted_unverified",
			wantAll:    "accepted_unverified",
		},
		{
			name:    "open ids confirmed exactly",
			request: MessageMentionRequest{IDs: []string{"ou_alpha", "ou_beta"}},
			response: []any{
				map[string]any{"key": "@_user_1", "id": "ou_alpha", "id_type": "open_id"},
				map[string]any{"key": "@_user_2", "id": "ou_beta", "id_type": "open_id"},
			},
			wantStatus:    "complete",
			wantConfirmed: 2,
			wantAll:       "not_requested",
		},
		{
			name:    "missing open id stays unattributed",
			request: MessageMentionRequest{IDs: []string{"ou_alpha", "ou_beta"}},
			response: []any{
				map[string]any{"key": "@_user_1", "id": "ou_alpha", "id_type": "open_id"},
			},
			wantStatus:    "partial_unattributed",
			wantConfirmed: 1,
			wantUnattrib:  []string{"ou_beta"},
			wantAll:       "not_requested",
		},
		{
			name:    "normalized user id cannot be guessed",
			request: MessageMentionRequest{IDs: []string{"u_alpha"}},
			response: []any{
				map[string]any{"key": "@_user_1", "id": "ou_normalized", "id_type": "open_id"},
			},
			wantStatus:   "partial_unattributed",
			wantUnattrib: []string{"u_alpha"},
			wantAll:      "not_requested",
		},
		{
			name:    "unknown response evidence is unattributed",
			request: MessageMentionRequest{IDs: []string{"ou_alpha"}},
			response: []any{
				map[string]any{"key": "@_user_1", "id": "ou_unknown", "id_type": "open_id"},
			},
			wantStatus:   "partial_unattributed",
			wantUnattrib: []string{"ou_alpha"},
			wantAll:      "not_requested",
		},
		{
			name:    "duplicate response key is unattributed",
			request: MessageMentionRequest{IDs: []string{"ou_alpha", "ou_beta"}},
			response: []any{
				map[string]any{"key": "@_user_1", "id": "ou_alpha", "id_type": "open_id"},
				map[string]any{"key": "@_user_1", "id": "ou_beta", "id_type": "open_id"},
			},
			wantStatus:    "partial_unattributed",
			wantConfirmed: 1,
			wantUnattrib:  []string{"ou_beta"},
			wantAll:       "not_requested",
		},
		{
			name:    "extra unknown evidence invalidates otherwise complete mapping",
			request: MessageMentionRequest{IDs: []string{"ou_alpha"}},
			response: []any{
				map[string]any{"key": "@_user_1", "id": "ou_alpha", "id_type": "open_id"},
				map[string]any{"key": "@_user_2", "id": "ou_unknown", "id_type": "open_id"},
			},
			wantStatus:   "partial_unattributed",
			wantUnattrib: []string{"ou_alpha"},
			wantAll:      "not_requested",
		},
		{
			name:    "duplicate requested evidence invalidates otherwise complete mapping",
			request: MessageMentionRequest{IDs: []string{"ou_alpha"}},
			response: []any{
				map[string]any{"key": "@_user_1", "id": "ou_alpha", "id_type": "open_id"},
				map[string]any{"key": "@_user_2", "id": "ou_alpha", "id_type": "open_id"},
			},
			wantStatus:   "partial_unattributed",
			wantUnattrib: []string{"ou_alpha"},
			wantAll:      "not_requested",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildMessageMentionResult(tt.request, tt.response)
			if got.Status != tt.wantStatus {
				t.Fatalf("status = %v, want %q", got.Status, tt.wantStatus)
			}
			if got.RetryScope != "none" {
				t.Fatalf("retry_scope = %v, want none", got.RetryScope)
			}
			if got.All != tt.wantAll {
				t.Fatalf("all = %v, want %q", got.All, tt.wantAll)
			}
			if len(got.Confirmed) != tt.wantConfirmed {
				t.Fatalf("confirmed = %#v, want len %d", got.Confirmed, tt.wantConfirmed)
			}
			assertStringSlice(t, got.Missing, tt.wantMissing)
			assertStringSlice(t, got.UnattributedRequested, tt.wantUnattrib)
		})
	}
}

func TestFinalizeMessageMentionResult(t *testing.T) {
	contract, ok := Lookup("im +messages-send")
	if !ok {
		t.Fatal("messages-send contract missing")
	}

	tests := []struct {
		name     string
		mention  any
		wantOK   bool
		wantExit int
		wantErr  bool
	}{
		{name: "absent stays compatible", wantOK: true},
		{name: "complete", mention: validMentionResult("complete"), wantOK: true},
		{name: "accepted all", mention: validMentionResult("accepted_unverified"), wantOK: true},
		{name: "partial", mention: validMentionResult("partial"), wantExit: output.ExitAPI},
		{name: "partial unattributed", mention: validMentionResult("partial_unattributed"), wantExit: output.ExitAPI},
		{name: "unknown status", mention: validMentionResult("mystery"), wantErr: true},
		{name: "replay scope cannot authorize replay", mention: MessageMentionResult{
			Status: "partial", Requested: []string{"ou_a"}, Confirmed: []MessageMentionConfirmation{},
			Missing: []string{"ou_a"}, All: "not_requested", RetryScope: "whole_request",
		}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{"message_id": "om_result"}
			if tt.mention != nil {
				data["mention_result"] = tt.mention
			}
			got, err := NewSession(contract).FinalizeSuccess(data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FinalizeSuccess() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got.OK != tt.wantOK || got.ExitCode != tt.wantExit {
				t.Fatalf("result = %#v, want ok=%v exit=%d", got, tt.wantOK, tt.wantExit)
			}
		})
	}
}

func validMentionResult(status string) MessageMentionResult {
	result := MessageMentionResult{
		Status:     status,
		Requested:  []string{},
		Confirmed:  []MessageMentionConfirmation{},
		Missing:    []string{},
		All:        "not_requested",
		RetryScope: "none",
	}
	switch status {
	case "accepted_unverified":
		result.All = "accepted_unverified"
	case "partial":
		result.Requested = []string{"ou_a"}
		result.Missing = []string{"ou_a"}
	case "partial_unattributed":
		result.Requested = []string{"u_a"}
		result.UnattributedRequested = []string{"u_a"}
	}
	return result
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("value = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("value = %#v, want %#v", got, want)
		}
	}
}
