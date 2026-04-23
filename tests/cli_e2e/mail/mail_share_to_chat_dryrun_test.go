// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestMail_ShareToChatDryRun validates the request shape emitted by
// +share-to-chat under --dry-run: the full CLI binary is invoked end-to-end
// so flag parsing, validation, and the dry-run renderer all execute.
// Fake credentials are sufficient because --dry-run short-circuits before
// any network call.
func TestMail_ShareToChatDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name     string
		args     []string
		wantURLs []string
	}{
		{
			name: "message-id with default chat_id",
			args: []string{
				"mail", "+share-to-chat",
				"--message-id", "msg_001",
				"--receive-id", "oc_xxx",
				"--dry-run",
			},
			wantURLs: []string{
				"/open-apis/mail/v1/user_mailboxes/me/messages/share_token",
				"/open-apis/mail/v1/user_mailboxes/me/share_tokens/%3Ccard_id%3E/send",
			},
		},
		{
			name: "thread-id with email type",
			args: []string{
				"mail", "+share-to-chat",
				"--thread-id", "thread_001",
				"--receive-id", "user@example.com",
				"--receive-id-type", "email",
				"--dry-run",
			},
			wantURLs: []string{
				"/open-apis/mail/v1/user_mailboxes/me/messages/share_token",
				"/open-apis/mail/v1/user_mailboxes/me/share_tokens/%3Ccard_id%3E/send",
			},
		},
		{
			name: "custom mailbox",
			args: []string{
				"mail", "+share-to-chat",
				"--message-id", "msg_002",
				"--receive-id", "oc_xxx",
				"--mailbox", "alias@example.com",
				"--dry-run",
			},
			wantURLs: []string{
				"/open-apis/mail/v1/user_mailboxes/alias@example.com/messages/share_token",
				"/open-apis/mail/v1/user_mailboxes/alias@example.com/share_tokens/%3Ccard_id%3E/send",
			},
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			for i, wantURL := range tt.wantURLs {
				gotMethod := gjson.Get(out, "api."+string(rune('0'+i))+".method").String()
				gotURL := gjson.Get(out, "api."+string(rune('0'+i))+".url").String()
				if gotMethod != "POST" {
					t.Fatalf("api[%d].method = %q, want POST\nstdout:\n%s", i, gotMethod, out)
				}
				if gotURL != wantURL {
					t.Fatalf("api[%d].url = %q, want %q\nstdout:\n%s", i, gotURL, wantURL, out)
				}
			}
		})
	}
}
