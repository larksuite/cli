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
	} {
		cmd.Flags().String(name, "", "")
	}
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
