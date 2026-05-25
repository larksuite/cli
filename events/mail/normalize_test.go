// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/event"
)

type fakeRT struct {
	getMailboxResponse json.RawMessage
	getMailboxErr      error
	gotPath            string
}

func (f *fakeRT) CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	f.gotPath = path
	if f.getMailboxErr != nil {
		return nil, f.getMailboxErr
	}
	return f.getMailboxResponse, nil
}

func TestNormalizeMailParams(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		response    json.RawMessage
		responseErr error
		wantOut     string
		wantErrSub  string
		wantAPICall bool
	}{
		{
			name:        "me resolves to real email",
			input:       "me",
			response:    json.RawMessage(`{"data":{"primary_email_address":"liuxinyang@example.com"}}`),
			wantOut:     "liuxinyang@example.com",
			wantAPICall: true,
		},
		{
			name:        "empty resolves to real email",
			input:       "",
			response:    json.RawMessage(`{"data":{"primary_email_address":"liuxinyang@example.com"}}`),
			wantOut:     "liuxinyang@example.com",
			wantAPICall: true,
		},
		{
			name:        "trim whitespace, no API call",
			input:       "  user@example.com  ",
			wantOut:     "user@example.com",
			wantAPICall: false,
		},
		{
			name:        "explicit email passes through",
			input:       "user@example.com",
			wantOut:     "user@example.com",
			wantAPICall: false,
		},
		{
			name:        "API error wraps with context",
			input:       "me",
			responseErr: errors.New("network down"),
			wantErrSub:  "resolve mailbox 'me': network down",
		},
		{
			name:       "empty email in response is error",
			input:      "me",
			response:   json.RawMessage(`{"data":{"primary_email_address":""}}`),
			wantErrSub: "empty primary_email_address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &fakeRT{getMailboxResponse: tt.response, getMailboxErr: tt.responseErr}
			params := map[string]string{"mailbox": tt.input}
			err := normalizeMailParams(context.Background(), rt, params)
			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("want error containing %q, got %v", tt.wantErrSub, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if params["mailbox"] != tt.wantOut {
				t.Errorf("got mailbox=%q want %q", params["mailbox"], tt.wantOut)
			}
			if tt.wantAPICall && rt.gotPath == "" {
				t.Error("expected API call but none made")
			}
			if !tt.wantAPICall && rt.gotPath != "" {
				t.Errorf("unexpected API call to %s", rt.gotPath)
			}
		})
	}
}

// Compile-time: normalizeMailParams must match the NormalizeParams signature.
var _ func(context.Context, event.APIClient, map[string]string) error = normalizeMailParams
