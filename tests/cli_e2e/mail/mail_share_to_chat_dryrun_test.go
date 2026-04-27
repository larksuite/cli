// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"strconv"
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
		name           string
		args           []string
		wantURLs       []string
		wantCreateBody map[string]string
		wantSendBody   map[string]string
		wantSendParams map[string]string
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
			wantCreateBody: map[string]string{"message_id": "msg_001"},
			wantSendBody:   map[string]string{"receive_id": "oc_xxx"},
			wantSendParams: map[string]string{"receive_id_type": "chat_id"},
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
			wantCreateBody: map[string]string{"thread_id": "thread_001"},
			wantSendBody:   map[string]string{"receive_id": "user@example.com"},
			wantSendParams: map[string]string{"receive_id_type": "email"},
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
			wantCreateBody: map[string]string{"message_id": "msg_002"},
			wantSendBody:   map[string]string{"receive_id": "oc_xxx"},
			wantSendParams: map[string]string{"receive_id_type": "chat_id"},
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
			gotCount := int(gjson.Get(out, "api.#").Int())
			if gotCount != len(tt.wantURLs) {
				t.Fatalf("expected %d API calls, got %d\nstdout:\n%s", len(tt.wantURLs), gotCount, out)
			}
			for i, wantURL := range tt.wantURLs {
				idx := strconv.Itoa(i)
				gotMethod := gjson.Get(out, "api."+idx+".method").String()
				gotURL := gjson.Get(out, "api."+idx+".url").String()
				if gotMethod != "POST" {
					t.Fatalf("api[%d].method = %q, want POST\nstdout:\n%s", i, gotMethod, out)
				}
				if gotURL != wantURL {
					t.Fatalf("api[%d].url = %q, want %q\nstdout:\n%s", i, gotURL, wantURL, out)
				}
			}

			for k, v := range tt.wantCreateBody {
				got := gjson.Get(out, "api.0.body."+k).String()
				if got != v {
					t.Fatalf("api[0].body.%s = %q, want %q\nstdout:\n%s", k, got, v, out)
				}
			}
			for k, v := range tt.wantSendBody {
				got := gjson.Get(out, "api.1.body."+k).String()
				if got != v {
					t.Fatalf("api[1].body.%s = %q, want %q\nstdout:\n%s", k, got, v, out)
				}
			}
			for k, v := range tt.wantSendParams {
				got := gjson.Get(out, "api.1.params."+k).String()
				if got != v {
					t.Fatalf("api[1].params.%s = %q, want %q\nstdout:\n%s", k, got, v, out)
				}
			}
		})
	}
}
