// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import "testing"

func TestShortcuts_Registration(t *testing.T) {
	shortcuts := Shortcuts()
	if len(shortcuts) != 1 {
		t.Fatalf("expected 1 shortcut, got %d", len(shortcuts))
	}
	s := shortcuts[0]
	if s.Service != "feed" {
		t.Errorf("expected Service=%q, got %q", "feed", s.Service)
	}
	if s.Command != "+sensitive" {
		t.Errorf("expected Command=%q, got %q", "+sensitive", s.Command)
	}
}
