// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/spf13/cobra"
)

// newDraftEditRuntime creates a minimal RuntimeContext with the draft-edit
// flags used by buildDraftEditPatch.
func newDraftEditRuntime(flags map[string]string) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	for _, name := range []string{
		"set-subject", "set-to", "set-cc", "set-bcc",
		"set-priority", "patch-file",
		"set-event-summary", "set-event-start", "set-event-end", "set-event-location",
	} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Bool("remove-event", false, "")
	for name, val := range flags {
		_ = cmd.Flags().Set(name, val)
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func TestBuildDraftEditPatch_SetPriorityHigh(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{"set-priority": "high"})
	patch, err := buildDraftEditPatch(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patch.Ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(patch.Ops))
	}
	op := patch.Ops[0]
	if op.Op != "set_header" {
		t.Errorf("Op = %q, want set_header", op.Op)
	}
	if op.Name != "X-Cli-Priority" {
		t.Errorf("Name = %q, want X-Cli-Priority", op.Name)
	}
	if op.Value != "1" {
		t.Errorf("Value = %q, want 1", op.Value)
	}
}

func TestBuildDraftEditPatch_SetPriorityLow(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{"set-priority": "low"})
	patch, err := buildDraftEditPatch(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patch.Ops) != 1 || patch.Ops[0].Value != "5" {
		t.Fatalf("expected single set_header with value 5, got %+v", patch.Ops)
	}
}

func TestBuildDraftEditPatch_SetPriorityNormalClears(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{"set-priority": "normal"})
	patch, err := buildDraftEditPatch(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patch.Ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(patch.Ops))
	}
	if patch.Ops[0].Op != "remove_header" || patch.Ops[0].Name != "X-Cli-Priority" {
		t.Errorf("expected remove_header X-Cli-Priority, got %+v", patch.Ops[0])
	}
}

func TestBuildDraftEditPatch_InvalidPriority(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{"set-priority": "urgent"})
	if _, err := buildDraftEditPatch(rt); err == nil {
		t.Fatal("expected error for invalid --set-priority value")
	}
}

func TestBuildDraftEditPatch_NoPriority(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{"set-subject": "hello"})
	patch, err := buildDraftEditPatch(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the set_subject op should be present; no priority op injected.
	if len(patch.Ops) != 1 || patch.Ops[0].Op != "set_subject" {
		t.Errorf("expected single set_subject op, got %+v", patch.Ops)
	}
}

func TestPrettyDraftAddresses(t *testing.T) {
	tests := []struct {
		name  string
		addrs []draftpkg.Address
		want  string
	}{
		{"empty", nil, ""},
		{"single address only", []draftpkg.Address{{Address: "a@b.com"}}, "a@b.com"},
		{"single with name", []draftpkg.Address{{Name: "Alice", Address: "a@b.com"}}, `"Alice" <a@b.com>`},
		{"multiple", []draftpkg.Address{
			{Address: "a@b.com"},
			{Name: "Bob", Address: "b@c.com"},
		}, `a@b.com, "Bob" <b@c.com>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prettyDraftAddresses(tt.addrs)
			if got != tt.want {
				t.Errorf("prettyDraftAddresses() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDraftEditPatch_SetEventEmitsSetCalendarOp(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{
		"set-event-summary":  "Team Sync",
		"set-event-start":    "2026-05-10T10:00:00+08:00",
		"set-event-end":      "2026-05-10T11:00:00+08:00",
		"set-event-location": "Room 301",
	})
	patch, err := buildDraftEditPatch(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patch.Ops) != 1 {
		t.Fatalf("expected 1 op, got %d: %+v", len(patch.Ops), patch.Ops)
	}
	op := patch.Ops[0]
	if op.Op != "set_calendar" {
		t.Errorf("Op = %q, want set_calendar", op.Op)
	}
	if op.EventSummary != "Team Sync" {
		t.Errorf("EventSummary = %q, want Team Sync", op.EventSummary)
	}
	if op.EventLocation != "Room 301" {
		t.Errorf("EventLocation = %q, want Room 301", op.EventLocation)
	}
}

func TestBuildDraftEditPatch_RemoveEventEmitsRemoveCalendarOp(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{
		"remove-event": "true",
	})
	patch, err := buildDraftEditPatch(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patch.Ops) != 1 || patch.Ops[0].Op != "remove_calendar" {
		t.Fatalf("expected single remove_calendar op, got %+v", patch.Ops)
	}
}

func TestBuildDraftEditPatch_SetAndRemoveEventMutuallyExclusive(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{
		"set-event-summary": "Meeting",
		"remove-event":      "true",
	})
	_, err := buildDraftEditPatch(rt)
	if err == nil {
		t.Fatal("expected error for --set-event-summary + --remove-event, got nil")
	}
}

func TestBuildDraftEditPatch_SetEventMissingStartEnd(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{
		"set-event-summary": "Meeting",
	})
	_, err := buildDraftEditPatch(rt)
	if err == nil {
		t.Fatal("expected error when --set-event-summary set without start/end, got nil")
	}
}

func TestEffectiveRecipients_SetReplaces(t *testing.T) {
	snapshot := &draftpkg.DraftSnapshot{
		To: []draftpkg.Address{{Address: "old@example.com"}},
		Cc: []draftpkg.Address{{Address: "cc@example.com"}},
	}
	ops := []draftpkg.PatchOp{
		{Op: "set_recipients", Field: "to", Addresses: []draftpkg.Address{{Address: "new@example.com"}}},
	}
	to, cc := effectiveRecipients(snapshot, ops)
	if len(to) != 1 || to[0].Address != "new@example.com" {
		t.Errorf("expected to=[new@example.com], got %v", to)
	}
	if len(cc) != 1 || cc[0].Address != "cc@example.com" {
		t.Errorf("expected cc unchanged, got %v", cc)
	}
}

func TestEffectiveRecipients_AddAndRemove(t *testing.T) {
	snapshot := &draftpkg.DraftSnapshot{
		To: []draftpkg.Address{{Address: "alice@example.com"}, {Address: "bob@example.com"}},
	}
	ops := []draftpkg.PatchOp{
		{Op: "add_recipient", Field: "to", Address: "carol@example.com"},
		{Op: "remove_recipient", Field: "to", Address: "bob@example.com"},
	}
	to, _ := effectiveRecipients(snapshot, ops)
	if len(to) != 2 {
		t.Fatalf("expected 2 recipients, got %v", to)
	}
	addrs := map[string]bool{}
	for _, a := range to {
		addrs[a.Address] = true
	}
	if !addrs["alice@example.com"] || !addrs["carol@example.com"] || addrs["bob@example.com"] {
		t.Errorf("unexpected recipient set: %v", to)
	}
}

func TestEffectiveRecipients_NoOpsReturnsCopy(t *testing.T) {
	snapshot := &draftpkg.DraftSnapshot{
		To: []draftpkg.Address{{Address: "alice@example.com"}},
		Cc: []draftpkg.Address{{Address: "bob@example.com"}},
	}
	to, cc := effectiveRecipients(snapshot, nil)
	if len(to) != 1 || to[0].Address != "alice@example.com" {
		t.Errorf("unexpected to: %v", to)
	}
	if len(cc) != 1 || cc[0].Address != "bob@example.com" {
		t.Errorf("unexpected cc: %v", cc)
	}
}
