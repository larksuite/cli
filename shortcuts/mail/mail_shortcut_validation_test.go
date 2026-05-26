// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

// assertValidationError fails the test unless err carries the validation
// category with ExitValidation exit code and a message containing wantSubstr.
// Accepts both typed *errs.ValidationError and legacy *output.ExitError so
// the helper survives the error-contract migration.
func assertValidationError(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
	// Accept both typed *errs.ValidationError and legacy *output.ExitError —
	// the helper's purpose is to assert "this is a validation-category
	// error" via either contract, so the dual-path matches the docstring.
	code := output.ExitCodeOf(err)
	if !errs.IsValidation(err) && code != output.ExitValidation {
		t.Fatalf("expected a validation-category error, got %T: %v", err, err)
	}
	if code != output.ExitValidation {
		t.Errorf("expected exit code %d (ExitValidation), got %d", output.ExitValidation, code)
	}
	if wantSubstr != "" && !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("expected error message to contain %q, got: %v", wantSubstr, err.Error())
	}
}

// assertValidatePasses fails the test if err is a validation error; other
// errors (e.g. API call failures from missing tokens) are acceptable because
// we only care that the Validate callback passed.
func assertValidatePasses(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if errs.IsValidation(err) || output.ExitCodeOf(err) == output.ExitValidation {
		t.Fatalf("Validate callback should have passed but returned validation error: %v", err)
	}
	// Non-validation errors (auth/API failures) are expected without HTTP mocks.
}

// TC-1: +message --as bot --mailbox me → ErrValidation
func TestMailMessageBotMailboxMeReturnsValidationError(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessage, []string{
		"+message", "--as", "bot", "--mailbox", "me", "--message-id", "msg_xxx",
	}, f, stdout)
	assertValidationError(t, err, "does not support --mailbox me")
}

// TC-2: +message --as bot --mailbox explicit → Validate passes
func TestMailMessageBotExplicitMailboxPassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessage, []string{
		"+message", "--as", "bot", "--mailbox", "alice@example.com", "--message-id", "msg_xxx",
	}, f, stdout)
	assertValidatePasses(t, err)
}

// TC-3: +message --as user --mailbox me → Validate passes
func TestMailMessageUserMailboxMePassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessage, []string{
		"+message", "--as", "user", "--mailbox", "me", "--message-id", "msg_xxx",
	}, f, stdout)
	assertValidatePasses(t, err)
}

// TC-4: +messages --as bot (default mailbox=me) → ErrValidation
func TestMailMessagesBotDefaultMailboxMeReturnsValidationError(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessages, []string{
		"+messages", "--as", "bot", "--message-ids", "msg_xxx",
	}, f, stdout)
	assertValidationError(t, err, "does not support --mailbox me")
}

// TC-5: +messages --as bot --mailbox explicit → Validate passes
func TestMailMessagesBotExplicitMailboxPassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailMessages, []string{
		"+messages", "--as", "bot", "--mailbox", "alice@example.com", "--message-ids", "msg_xxx",
	}, f, stdout)
	assertValidatePasses(t, err)
}

// TC-6: +thread --as bot (default mailbox=me) → ErrValidation
func TestMailThreadBotDefaultMailboxMeReturnsValidationError(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailThread, []string{
		"+thread", "--as", "bot", "--thread-id", "thread_xxx",
	}, f, stdout)
	assertValidationError(t, err, "does not support --mailbox me")
}

// TC-7: +thread --as bot --mailbox explicit → Validate passes
func TestMailThreadBotExplicitMailboxPassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailThread, []string{
		"+thread", "--as", "bot", "--mailbox", "alice@example.com", "--thread-id", "thread_xxx",
	}, f, stdout)
	assertValidatePasses(t, err)
}

// TC-8: +triage --as bot (default mailbox=me) → ErrValidation
func TestMailTriageBotDefaultMailboxMeReturnsValidationError(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--as", "bot",
	}, f, stdout)
	assertValidationError(t, err, "does not support --mailbox me")
}

// TC-9: +triage --as bot --mailbox explicit → Validate passes
func TestMailTriageBotExplicitMailboxPassesValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--as", "bot", "--mailbox", "alice@example.com",
	}, f, stdout)
	assertValidatePasses(t, err)
}
