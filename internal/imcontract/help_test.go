// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestHelpPolicyTextUsesOnlyApprovedTemplates(t *testing.T) {
	tests := []struct {
		policy HelpPolicy
		want   string
	}{
		{HelpCompleteness, "Completeness: use --page-all --page-limit 0 for exhaustive output; only meta.pagination.complete=true proves pagination completion."},
		{HelpAcceptanceOnly, "Verify the final state with lark-cli im chat.moderation get --chat-id <same_chat_id> --as <same_identity>."},
		{HelpPolicy("unknown"), ""},
	}
	for _, tt := range tests {
		if got := tt.policy.Text(); got != tt.want {
			t.Fatalf("HelpPolicy(%q).Text() = %q, want %q", tt.policy, got, tt.want)
		}
	}
}

func TestRegistryHelpPolicies(t *testing.T) {
	tests := []struct {
		key  ContractKey
		want HelpPolicy
	}{
		{"im +chat-list", HelpCompleteness},
		{"im +messages-search", HelpCompleteness},
		{"im chat.moderation get", HelpCompleteness},
		{"im +messages-send", ""},
		{"im messages merge_forward", ""},
		{"im chat.moderation update", HelpAcceptanceOnly},
		{"im +flag-create", ""},
	}
	for _, tt := range tests {
		contract, ok := Lookup(tt.key)
		if !ok {
			t.Fatalf("missing contract %q", tt.key)
		}
		if contract.HelpPolicy != tt.want {
			t.Fatalf("%s HelpPolicy = %q, want %q", tt.key, contract.HelpPolicy, tt.want)
		}
	}
}

func TestHelpTextIsLazyAndRunnableOnly(t *testing.T) {
	cmd := &cobra.Command{Use: "+chat-list", Short: "List chats", Run: func(*cobra.Command, []string) {}}
	AnnotateHelpContract(cmd, "im +chat-list")
	if cmd.Long != "" || cmd.Short != "List chats" {
		t.Fatalf("annotation changed visible help fields: Short=%q Long=%q", cmd.Short, cmd.Long)
	}
	if got := HelpText(cmd); got != HelpCompleteness.Text() {
		t.Fatalf("HelpText() = %q", got)
	}
	parent := &cobra.Command{Use: "im"}
	AnnotateHelpContract(parent, "im +chat-list")
	if got := HelpText(parent); got != "" {
		t.Fatalf("parent HelpText() = %q, want empty", got)
	}
}

func TestHelpTextAddsSameKeyReplayOnlyToApplicableCommands(t *testing.T) {
	const approvedSameKeyText = "Idempotent retry: generate the key outside this command, then reuse the same literal with unchanged parameters on every retry."
	if helpSameKeyReplay != approvedSameKeyText {
		t.Fatalf("same-key help = %q, want approved text %q", helpSameKeyReplay, approvedSameKeyText)
	}
	tests := []struct {
		key  ContractKey
		want string
	}{
		{"im +messages-send", approvedSameKeyText},
		{"im +chat-create", approvedSameKeyText},
		{"im +chat-update", ""},
	}
	for _, tt := range tests {
		cmd := &cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}}
		AnnotateHelpContract(cmd, tt.key)
		if got := HelpText(cmd); got != tt.want {
			t.Fatalf("%s HelpText() = %q, want %q", tt.key, got, tt.want)
		}
	}
}
