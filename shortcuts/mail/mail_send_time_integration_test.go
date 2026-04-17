// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// mailShortcutTestFactoryWithSendScope mirrors mailShortcutTestFactory but
// additionally grants the mail:user_mailbox.message:send scope so tests can
// exercise code paths guarded by validateConfirmSendScope (e.g. validateSendTime).
func mailShortcutTestFactoryWithSendScope(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())

	cfg := mailTestConfig()
	token := &auth.StoredUAToken{
		UserOpenId:       cfg.UserOpenId,
		AppId:            cfg.AppID,
		AccessToken:      "test-user-access-token",
		RefreshToken:     "test-refresh-token",
		ExpiresAt:        time.Now().Add(1 * time.Hour).UnixMilli(),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli(),
		Scope:            "mail:user_mailbox.messages:write mail:user_mailbox.messages:read mail:user_mailbox.message:modify mail:user_mailbox.message:send mail:user_mailbox.message:readonly mail:user_mailbox.message.address:read mail:user_mailbox.message.subject:read mail:user_mailbox.message.body:read mail:user_mailbox:readonly",
		GrantedAt:        time.Now().Add(-1 * time.Hour).UnixMilli(),
	}
	if err := auth.SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	t.Cleanup(func() {
		_ = auth.RemoveStoredToken(cfg.AppID, cfg.UserOpenId)
	})
	return cmdutil.TestFactory(t, cfg)
}

// tooSoonSendTime returns a send-time 60s in the future — below the 5-minute
// floor enforced by validateSendTime.
func tooSoonSendTime() string {
	return strconv.FormatInt(time.Now().Unix()+60, 10)
}

// futureSendTime returns a send-time 10 minutes in the future — above the floor.
func futureSendTime() string {
	return strconv.FormatInt(time.Now().Unix()+10*60, 10)
}

// ---------------------------------------------------------------------------
// Invalid --send-time rejected by each compose shortcut
// ---------------------------------------------------------------------------

func TestMailSend_SendTimeTooSoon(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactoryWithSendScope(t)
	err := runMountedMailShortcut(t, MailSend, []string{
		"+send", "--to", "alice@example.com", "--subject", "hi", "--body", "hello",
		"--confirm-send", "--send-time", tooSoonSendTime(),
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for too-soon send-time, got nil")
	}
	if !strings.Contains(err.Error(), "5 minutes") {
		t.Errorf("expected 5-minute error, got: %v", err)
	}
}

func TestMailReply_SendTimeTooSoon(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactoryWithSendScope(t)
	err := runMountedMailShortcut(t, MailReply, []string{
		"+reply", "--message-id", "msg_001", "--body", "hello",
		"--confirm-send", "--send-time", tooSoonSendTime(),
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for too-soon send-time, got nil")
	}
	if !strings.Contains(err.Error(), "5 minutes") {
		t.Errorf("expected 5-minute error, got: %v", err)
	}
}

func TestMailReplyAll_SendTimeTooSoon(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactoryWithSendScope(t)
	err := runMountedMailShortcut(t, MailReplyAll, []string{
		"+reply-all", "--message-id", "msg_001", "--body", "hello",
		"--confirm-send", "--send-time", tooSoonSendTime(),
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for too-soon send-time, got nil")
	}
	if !strings.Contains(err.Error(), "5 minutes") {
		t.Errorf("expected 5-minute error, got: %v", err)
	}
}

func TestMailForward_SendTimeTooSoon(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactoryWithSendScope(t)
	err := runMountedMailShortcut(t, MailForward, []string{
		"+forward", "--message-id", "msg_001", "--to", "alice@example.com",
		"--confirm-send", "--send-time", tooSoonSendTime(),
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for too-soon send-time, got nil")
	}
	if !strings.Contains(err.Error(), "5 minutes") {
		t.Errorf("expected 5-minute error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// --send-time without --confirm-send is rejected up front
// ---------------------------------------------------------------------------

func TestMailSend_SendTimeWithoutConfirmSend(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactoryWithSendScope(t)
	err := runMountedMailShortcut(t, MailSend, []string{
		"+send", "--to", "alice@example.com", "--subject", "hi", "--body", "hello",
		"--send-time", futureSendTime(),
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error for --send-time without --confirm-send, got nil")
	}
	if !strings.Contains(err.Error(), "--confirm-send") {
		t.Errorf("expected error to mention --confirm-send, got: %v", err)
	}
}
