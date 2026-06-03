// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestChatID_ValidatePass(t *testing.T) {
	id := ChatID("oc_abc123")
	if err := id.ValidateValue(nil, "chat-id"); err != nil {
		t.Errorf("oc_ prefix should pass, got: %v", err)
	}
}

func TestChatID_ValidateReject(t *testing.T) {
	tests := []struct {
		name string
		v    string
	}{
		{"empty", ""},
		{"wrong prefix", "ou_abc"},
		{"random", "abc123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ChatID(tt.v).ValidateValue(nil, "chat-id")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *errs.ValidationError, got %T", err)
			}
			if ve.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("Subtype = %q, want invalid_argument", ve.Subtype)
			}
			if ve.Param != "chat-id" {
				t.Errorf("Param = %q, want chat-id", ve.Param)
			}
		})
	}
}
