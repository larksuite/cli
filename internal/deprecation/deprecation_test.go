// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deprecation

import (
	"strings"
	"testing"
)

func TestNoticeMessage(t *testing.T) {
	tests := []struct {
		name   string
		notice Notice
		want   string
	}{
		{
			name:   "replacement and skill",
			notice: Notice{Command: "+read", Replacement: "+cells-get", Skill: "lark-sheets"},
			want:   "+read is a pre-refactor compatibility alias; use +cells-get instead; update your lark-sheets skill, run: lark-cli update",
		},
		{
			name:   "no replacement",
			notice: Notice{Command: "+read", Skill: "lark-sheets"},
			want:   "+read is a pre-refactor compatibility alias; update your lark-sheets skill, run: lark-cli update",
		},
		{
			name:   "no skill",
			notice: Notice{Command: "+read", Replacement: "+cells-get"},
			want:   "+read is a pre-refactor compatibility alias; use +cells-get instead; update your skill, run: lark-cli update",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.notice.Message(); got != tt.want {
				t.Errorf("Message() =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestNoticeMessageWithoutUpdateAction(t *testing.T) {
	n := &Notice{
		Command:     "+read",
		Replacement: "+cells-get",
		Skill:       "lark-sheets",
	}
	got := n.MessageWithoutUpdateAction()
	want := "+read is a pre-refactor compatibility alias; use +cells-get instead; update your lark-sheets skill"
	if got != want {
		t.Fatalf("MessageWithoutUpdateAction() = %q, want %q", got, want)
	}
	if strings.Contains(got, "lark-cli update") {
		t.Fatalf("MessageWithoutUpdateAction() contains an update command: %q", got)
	}
}

func TestSetGetPending(t *testing.T) {
	t.Cleanup(func() { SetPending(nil) })

	SetPending(nil)
	if got := GetPending(); got != nil {
		t.Fatalf("expected nil pending after clear, got %#v", got)
	}

	n := &Notice{Command: "+write", Replacement: "+cells-set", Skill: "lark-sheets"}
	SetPending(n)
	got := GetPending()
	if got == nil || got.Command != "+write" || got.Replacement != "+cells-set" {
		t.Fatalf("GetPending() = %#v, want %#v", got, n)
	}

	SetPending(nil)
	if GetPending() != nil {
		t.Fatal("expected nil after clearing")
	}
}
