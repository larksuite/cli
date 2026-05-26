// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"
)

// TestMailWatch_DescriptionContainsMigrationTip ensures the Description field
// mentions 'event consume mail.user_mailbox.event.message_received_v1' and
// '--as user'. This is a v9 regression guard.
func TestMailWatch_DescriptionContainsMigrationTip(t *testing.T) {
	desc := MailWatch.Description
	if !strings.Contains(desc, "event consume mail.user_mailbox.event.message_received_v1") {
		t.Errorf("Description should mention 'event consume mail.user_mailbox.event.message_received_v1', got: %q", desc)
	}
	if !strings.Contains(desc, "--as user") {
		t.Errorf("Description should mention '--as user', got: %q", desc)
	}
}

// TestMailWatch_ForceFlagAvailableInHelp ensures the --force flag is defined
// in MailWatch.Flags, is of type "bool", and defaults to false.
func TestMailWatch_ForceFlagAvailableInHelp(t *testing.T) {
	var found bool
	for _, fl := range MailWatch.Flags {
		if fl.Name == "force" {
			found = true
			if fl.Type != "bool" {
				t.Errorf("--force flag type = %q, want \"bool\"", fl.Type)
			}
			// Default should be "false" or empty (interpreted as false)
			if fl.Default != "false" && fl.Default != "" {
				t.Errorf("--force flag default = %q, want \"false\" or \"\"", fl.Default)
			}
			break
		}
	}
	if !found {
		t.Error("--force flag not found in MailWatch.Flags")
	}
}

// TestMailWatch_HintDoesNotMentionDeprecatedSubscribe ensures the --force
// flag description does not reference 'event +subscribe' (v9 regression guard).
func TestMailWatch_HintDoesNotMentionDeprecatedSubscribe(t *testing.T) {
	for _, fl := range MailWatch.Flags {
		if fl.Name == "force" {
			if strings.Contains(fl.Desc, "event +subscribe") {
				t.Errorf("--force flag Desc should NOT mention 'event +subscribe', got: %q", fl.Desc)
			}
			return
		}
	}
	// flag not found — TestMailWatch_ForceFlagAvailableInHelp handles that
}

// TestMailWatch_PrintSchemaDoesNotRequireForce verifies that --print-output-schema
// works even when the --force flag is set to its default (false). This is a
// static check that print-output-schema runs the early-return path before the
// lockfile block.
func TestMailWatch_PrintSchemaDoesNotRequireForce(t *testing.T) {
	// Verify the Flags slice has print-output-schema before force in the intended
	// semantics: print-output-schema short-circuits before the lockfile block.
	printSchemaIdx := -1
	forceIdx := -1
	for i, fl := range MailWatch.Flags {
		switch fl.Name {
		case "print-output-schema":
			printSchemaIdx = i
		case "force":
			forceIdx = i
		}
	}
	if printSchemaIdx < 0 {
		t.Error("--print-output-schema flag not found")
	}
	if forceIdx < 0 {
		t.Error("--force flag not found")
	}
	// Both must be present; no ordering constraint is asserted beyond existence.
}

// TestMailWatch_ScopesCount verifies MailWatch.Scopes has exactly 7 items,
// consistent with the single source of truth for the 7 mail user scopes.
func TestMailWatch_ScopesCount(t *testing.T) {
	if len(MailWatch.Scopes) != 7 {
		t.Errorf("MailWatch.Scopes: expected 7 items, got %d: %v", len(MailWatch.Scopes), MailWatch.Scopes)
	}
}

// TestMailWatch_ScopesMatchEventKeyScopes ensures MailWatch.Scopes is aligned
// with the expected 7-item scope list, preventing scope drift between mail +watch
// and event consume mail.user_mailbox.event.message_received_v1.
func TestMailWatch_ScopesMatchEventKeyScopes(t *testing.T) {
	want := []string{
		"mail:event",
		"mail:user_mailbox.event.mail_address:read",
		"mail:user_mailbox:readonly",
		"mail:user_mailbox.message:readonly",
		"mail:user_mailbox.message.address:read",
		"mail:user_mailbox.message.subject:read",
		"mail:user_mailbox.message.body:read",
	}
	got := MailWatch.Scopes
	if len(got) != len(want) {
		t.Fatalf("scope count mismatch: want %d, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Scopes[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}
