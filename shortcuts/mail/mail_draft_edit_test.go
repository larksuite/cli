// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"os"
	"testing"

	"github.com/larksuite/cli/errs"
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
		"from", "set-priority", "patch-file",
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

func TestLoadPatchFileRejectsUnsafePathWithTypedParam(t *testing.T) {
	chdirTemp(t)
	f, _, _, _ := mailShortcutTestFactory(t)
	rt := &common.RuntimeContext{Cmd: &cobra.Command{Use: "test"}, Factory: f, Config: mailTestConfig()}
	_, err := loadPatchFile(rt, "../patch.json")
	if err == nil {
		t.Fatal("expected unsafe patch path to fail")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--patch-file" {
		t.Fatalf("param = %q, want --patch-file", validationErr.Param)
	}
}

func TestLoadPatchFileValidateFailureKeepsPatchFileParam(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("patch.json", []byte(`{"ops":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, _, _, _ := mailShortcutTestFactory(t)
	rt := &common.RuntimeContext{Cmd: &cobra.Command{Use: "test"}, Factory: f, Config: mailTestConfig()}
	_, err := loadPatchFile(rt, "patch.json")
	if err == nil {
		t.Fatal("expected invalid patch file to fail")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--patch-file" {
		t.Fatalf("param = %q, want --patch-file", validationErr.Param)
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

func TestBuildDraftEditPatch_SetFromHeader(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{"from": "Alias <alias@example.com>"})
	patch, err := buildDraftEditPatch(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patch.Ops) != 1 {
		t.Fatalf("expected 1 op, got %d: %+v", len(patch.Ops), patch.Ops)
	}
	op := patch.Ops[0]
	if op.Op != "set_header" || op.Name != "From" {
		t.Fatalf("expected set_header From, got %+v", op)
	}
	if op.Value != `"Alias" <alias@example.com>` {
		t.Fatalf("Value = %q, want %q", op.Value, `"Alias" <alias@example.com>`)
	}
}

func TestBuildDraftEditPatch_SetFromRejectsMultiple(t *testing.T) {
	rt := newDraftEditRuntime(map[string]string{"from": "a@example.com,b@example.com"})
	_, err := buildDraftEditPatch(rt)
	if err == nil {
		t.Fatal("expected error for multiple --from addresses")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--from" {
		t.Fatalf("param = %q, want --from", validationErr.Param)
	}
}

func TestBuildDraftEditPatch_PatchFileRejectsMultipleFrom(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("patch.json", []byte(`{"ops":[{"op":"set_header","name":"From","value":"Alice <alice@example.com>, Bob <bob@example.com>"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, _, _, _ := mailShortcutTestFactory(t)
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().String("set-subject", "", "")
	cmd.Flags().String("set-to", "", "")
	cmd.Flags().String("set-cc", "", "")
	cmd.Flags().String("set-bcc", "", "")
	cmd.Flags().String("set-priority", "", "")
	cmd.Flags().String("patch-file", "", "")
	cmd.Flags().Bool("allow-protected-header-edit", false, "")
	cmd.Flags().Bool("rewrite-entire-draft", false, "")
	cmd.Flags().String("body", "", "")
	cmd.Flags().String("body-file", "", "")
	cmd.Flags().String("set-event-summary", "", "")
	cmd.Flags().String("set-event-start", "", "")
	cmd.Flags().String("set-event-end", "", "")
	cmd.Flags().String("set-event-location", "", "")
	cmd.Flags().Bool("remove-event", false, "")
	cmd.Flags().Bool("request-receipt", false, "")
	_ = cmd.Flags().Set("patch-file", "patch.json")

	rt := &common.RuntimeContext{Cmd: cmd, Factory: f, Config: mailTestConfig()}
	_, err := buildDraftEditPatch(rt)
	if err == nil {
		t.Fatal("expected error for patch-file From with multiple addresses")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--patch-file" {
		t.Fatalf("param = %q, want --patch-file", validationErr.Param)
	}
}

func TestBuildDraftEditPatch_PatchFileRejectsDuplicateFromBeforeDedup(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("patch.json", []byte(`{"ops":[{"op":"set_header","name":"From","value":"Alice <alice@example.com>, Alias <ALICE@example.com>"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, _, _, _ := mailShortcutTestFactory(t)
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().String("set-subject", "", "")
	cmd.Flags().String("set-to", "", "")
	cmd.Flags().String("set-cc", "", "")
	cmd.Flags().String("set-bcc", "", "")
	cmd.Flags().String("set-priority", "", "")
	cmd.Flags().String("patch-file", "", "")
	cmd.Flags().Bool("allow-protected-header-edit", false, "")
	cmd.Flags().Bool("rewrite-entire-draft", false, "")
	cmd.Flags().String("body", "", "")
	cmd.Flags().String("body-file", "", "")
	cmd.Flags().String("set-event-summary", "", "")
	cmd.Flags().String("set-event-start", "", "")
	cmd.Flags().String("set-event-end", "", "")
	cmd.Flags().String("set-event-location", "", "")
	cmd.Flags().Bool("remove-event", false, "")
	cmd.Flags().Bool("request-receipt", false, "")
	_ = cmd.Flags().Set("patch-file", "patch.json")

	rt := &common.RuntimeContext{Cmd: cmd, Factory: f, Config: mailTestConfig()}
	_, err := buildDraftEditPatch(rt)
	if err == nil {
		t.Fatal("expected error for duplicate patch-file From entries")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--patch-file" {
		t.Fatalf("param = %q, want --patch-file", validationErr.Param)
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
