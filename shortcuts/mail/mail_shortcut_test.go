// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/ratelimit"
	"github.com/larksuite/cli/shortcuts/common"
)

func mailTestConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID:      "test-app",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_testuser",
		UserName:   "Test User",
	}
}

func mailShortcutTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	keyring.MockInit() // use in-memory keyring to avoid macOS keychain popups
	t.Setenv("HOME", t.TempDir())

	cfg := mailTestConfig()
	token := &auth.StoredUAToken{
		UserOpenId:       cfg.UserOpenId,
		AppId:            cfg.AppID,
		AccessToken:      "test-user-access-token",
		RefreshToken:     "test-refresh-token",
		ExpiresAt:        time.Now().Add(1 * time.Hour).UnixMilli(),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli(),
		Scope:            "mail:user_mailbox.messages:write mail:user_mailbox.messages:read mail:user_mailbox.message:modify mail:user_mailbox.message:readonly mail:user_mailbox.message.address:read mail:user_mailbox.message.subject:read mail:user_mailbox.message.body:read mail:user_mailbox:readonly",
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

func runMountedMailShortcut(t *testing.T, shortcut common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	shortcut.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func decodeShortcutEnvelopeData(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("Unmarshal(stdout) error = %v, stdout=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("expected ok output, stdout=%s", stdout.String())
	}
	return envelope.Data
}

func encodeFixtureEMLForMailTest(raw string) string {
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func TestMailMessageShortcutUsesLocalMailRateLimit(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	now := time.Unix(100, 0)
	rule := ratelimit.Rule{
		Method:        "GET",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id",
		Window:        2 * time.Second,
		Limit:         1,
		Scope:         ratelimit.ScopeApp,
	}
	restore := ratelimit.SetDefaultLimiterForTest(ratelimit.NewLimiterForDir(t.TempDir(), []ratelimit.Rule{rule}, func() time.Time { return now }))
	defer restore()

	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/messages/msg_1",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":      "msg_1",
					"subject":         "hello",
					"body_plain_text": encodeFixtureEMLForMailTest("hello"),
					"message_state":   "READ",
				},
			},
		},
	}
	reg.Register(stub)

	args := []string{"+message", "--message-id", "msg_1", "--html=false", "--as", "user"}
	if err := runMountedMailShortcut(t, MailMessage, args, f, stdout); err != nil {
		t.Fatalf("first +message err = %v", err)
	}
	if len(stub.CapturedBodies) != 1 {
		t.Fatalf("HTTP calls after first run = %d, want 1", len(stub.CapturedBodies))
	}

	err := runMountedMailShortcut(t, MailMessage, args, f, stdout)
	if err == nil {
		t.Fatal("expected local rate limit")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Type != "rate_limit" {
		t.Fatalf("err = %v, want rate_limit ExitError", err)
	}
	if len(stub.CapturedBodies) != 1 {
		t.Fatalf("HTTP calls after local rate limit = %d, want 1", len(stub.CapturedBodies))
	}
}

func TestMailMessagesShortcutUsesLocalMailRateLimit(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	now := time.Unix(100, 0)
	rule := ratelimit.Rule{
		Method:        "POST",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/batch_get",
		Window:        2 * time.Second,
		Limit:         1,
		Scope:         ratelimit.ScopeApp,
	}
	restore := ratelimit.SetDefaultLimiterForTest(ratelimit.NewLimiterForDir(t.TempDir(), []ratelimit.Rule{rule}, func() time.Time { return now }))
	defer restore()

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/messages/batch_get",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"messages": []interface{}{
					map[string]interface{}{
						"message_id":      "msg_1",
						"subject":         "hello",
						"body_plain_text": encodeFixtureEMLForMailTest("hello"),
						"message_state":   "READ",
					},
				},
			},
		},
	}
	reg.Register(stub)

	args := []string{"+messages", "--message-ids", "msg_1", "--html=false", "--as", "user"}
	if err := runMountedMailShortcut(t, MailMessages, args, f, stdout); err != nil {
		t.Fatalf("first +messages err = %v", err)
	}
	if len(stub.CapturedBodies) != 1 {
		t.Fatalf("HTTP calls after first run = %d, want 1", len(stub.CapturedBodies))
	}

	err := runMountedMailShortcut(t, MailMessages, args, f, stdout)
	if err == nil {
		t.Fatal("expected local rate limit")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Type != "rate_limit" {
		t.Fatalf("err = %v, want rate_limit ExitError", err)
	}
	if len(stub.CapturedBodies) != 1 {
		t.Fatalf("HTTP calls after local rate limit = %d, want 1", len(stub.CapturedBodies))
	}
}

func TestMailTriageShortcutPreservesLocalMailRateLimit(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	now := time.Unix(100, 0)
	rule := ratelimit.Rule{
		Method:        "POST",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/search",
		Window:        2 * time.Second,
		Limit:         1,
		Scope:         ratelimit.ScopeApp,
	}
	restore := ratelimit.SetDefaultLimiterForTest(ratelimit.NewLimiterForDir(t.TempDir(), []ratelimit.Rule{rule}, func() time.Time { return now }))
	defer restore()

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/search",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{},
				"has_more": false,
			},
		},
	}
	reg.Register(stub)

	args := []string{"+triage", "--query", "hello", "--format", "data", "--as", "user"}
	if err := runMountedMailShortcut(t, MailTriage, args, f, stdout); err != nil {
		t.Fatalf("first +triage err = %v", err)
	}
	if len(stub.CapturedBodies) != 1 {
		t.Fatalf("HTTP calls after first run = %d, want 1", len(stub.CapturedBodies))
	}

	err := runMountedMailShortcut(t, MailTriage, args, f, stdout)
	if err == nil {
		t.Fatal("expected local rate limit")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Type != "rate_limit" {
		t.Fatalf("err = %v, want rate_limit ExitError", err)
	}
	if len(stub.CapturedBodies) != 1 {
		t.Fatalf("HTTP calls after local rate limit = %d, want 1", len(stub.CapturedBodies))
	}
}

func TestMailWatchFetchMessageUsesLocalMailRateLimit(t *testing.T) {
	f, _, _, _ := mailShortcutTestFactory(t)
	now := time.Unix(100, 0)
	rule := ratelimit.Rule{
		Method:        "GET",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id",
		Window:        2 * time.Second,
		Limit:         1,
		Scope:         ratelimit.ScopeApp,
	}
	restore := ratelimit.SetDefaultLimiterForTest(ratelimit.NewLimiterForDir(t.TempDir(), []ratelimit.Rule{rule}, func() time.Time { return now }))
	defer restore()

	if err := ratelimit.Allow(context.Background(), ratelimit.Request{
		Brand:  core.BrandFeishu,
		AppID:  "test-app",
		Method: "GET",
		Path:   "/open-apis/mail/v1/user_mailboxes/me/messages/msg_1",
	}); err != nil {
		t.Fatalf("pre-consume rate limit slot err = %v", err)
	}

	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, mailTestConfig(), f, core.AsUser)
	_, err := fetchMessageForWatch(runtime, "me", "msg_1", "metadata")
	if err == nil {
		t.Fatal("expected local rate limit")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Type != "rate_limit" {
		t.Fatalf("err = %v, want rate_limit ExitError", err)
	}
}

// chdirTemp changes the working directory to a fresh temp directory and
// restores it when the test finishes. This allows SafeInputPath/SafeOutputPath
// to accept relative file paths created in the temp directory.
func chdirTemp(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}
