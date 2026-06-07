// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMail_SendDryRunSignatureRequestChain(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantURLs []string
	}{
		{
			name: "default signature lookup",
			args: []string{
				"mail", "+send",
				"--mailbox", "dry_box",
				"--to", "alice@example.com",
				"--subject", "Dry",
				"--body", "<p>Hello</p>",
				"--dry-run",
			},
			wantURLs: []string{
				"/open-apis/mail/v1/user_mailboxes/dry_box/profile",
				"/open-apis/mail/v1/user_mailboxes/dry_box/settings/signatures",
				"/open-apis/mail/v1/user_mailboxes/dry_box/settings/send_as",
				"/open-apis/mail/v1/user_mailboxes/dry_box/drafts",
			},
		},
		{
			name: "explicit signature then send",
			args: []string{
				"mail", "+send",
				"--mailbox", "dry_explicit_box",
				"--to", "alice@example.com",
				"--subject", "Dry",
				"--body", "<p>Hello</p>",
				"--signature-id", "sig_123",
				"--confirm-send",
				"--dry-run",
			},
			wantURLs: []string{
				"/open-apis/mail/v1/user_mailboxes/dry_explicit_box/profile",
				"/open-apis/mail/v1/user_mailboxes/dry_explicit_box/settings/signatures",
				"/open-apis/mail/v1/user_mailboxes/dry_explicit_box/settings/send_as",
				"/open-apis/mail/v1/user_mailboxes/dry_explicit_box/drafts",
				"/open-apis/mail/v1/user_mailboxes/dry_explicit_box/drafts/%3Cdraft_id%3E/send",
			},
		},
		{
			name: "skip signature",
			args: []string{
				"mail", "+send",
				"--mailbox", "dry_no_sig_box",
				"--to", "alice@example.com",
				"--subject", "Dry",
				"--body", "<p>Hello</p>",
				"--no-signature",
				"--dry-run",
			},
			wantURLs: []string{
				"/open-apis/mail/v1/user_mailboxes/dry_no_sig_box/profile",
				"/open-apis/mail/v1/user_mailboxes/dry_no_sig_box/drafts",
			},
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			setMailSendDryRunEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			require.Equal(t, int64(len(tt.wantURLs)), gjson.Get(result.Stdout, "api.#").Int(), "stdout:\n%s", result.Stdout)
			for i, wantURL := range tt.wantURLs {
				idx := strconv.Itoa(i)
				require.Equal(t, wantURL, gjson.Get(result.Stdout, "api."+idx+".url").String(), "stdout:\n%s", result.Stdout)
				if strings.HasSuffix(wantURL, "/drafts") {
					require.Equal(t, "<base64url-EML>", gjson.Get(result.Stdout, "api."+idx+".body.raw").String(), "stdout:\n%s", result.Stdout)
				}
			}
			require.Equal(t, "POST", gjson.Get(result.Stdout, "api."+strconv.Itoa(len(tt.wantURLs)-1)+".method").String(), "stdout:\n%s", result.Stdout)
		})
	}
}

func TestMail_SendDryRunRejectsSignatureSkipConflict(t *testing.T) {
	setMailSendDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"mail", "+send",
			"--to", "alice@example.com",
			"--subject", "Dry",
			"--body", "Hello",
			"--signature-id", "sig_123",
			"--no-signature",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stdout+result.Stderr, "--signature-id and --no-signature are mutually exclusive")
}

func setMailSendDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "mail_send_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "mail_send_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
