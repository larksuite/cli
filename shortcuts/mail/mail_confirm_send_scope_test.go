// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

func TestConfirmSendMissingScopeReply(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailReply, []string{
		"+reply", "--message-id", "msg_001", "--body", "hello", "--confirm-send",
	}, f, stdout)
	assertMissingSendScope(t, err)
}

func TestConfirmSendMissingScopeReplyAll(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailReplyAll, []string{
		"+reply-all", "--message-id", "msg_001", "--body", "hello", "--confirm-send",
	}, f, stdout)
	assertMissingSendScope(t, err)
}

func TestConfirmSendMissingScopeForward(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailForward, []string{
		"+forward", "--message-id", "msg_001", "--to", "alice@example.com", "--confirm-send",
	}, f, stdout)
	assertMissingSendScope(t, err)
}

func assertMissingSendScope(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error when token lacks send scope with --confirm-send, got nil")
	}
	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitAuth {
		t.Errorf("expected exit code %d (ExitAuth), got %d", output.ExitAuth, gotCode)
	}
	if permErr.Subtype != errs.SubtypeMissingScope {
		t.Errorf("expected subtype %q, got %q", errs.SubtypeMissingScope, permErr.Subtype)
	}
}
