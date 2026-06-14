// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMail_SendDryRunSignatureRequests(t *testing.T) {
	setMailDraftSendDryRunEnv(t)

	tests := []struct {
		name               string
		args               []string
		wantSignaturesGET  bool
		wantSignatureIndex int
	}{
		{
			name: "default signature path shows signatures GET",
			args: []string{
				"mail", "+send",
				"--mailbox", "me",
				"--to", "alice@example.com",
				"--subject", "dry run",
				"--body", "<p>Hello</p>",
				"--dry-run",
			},
			wantSignaturesGET:  true,
			wantSignatureIndex: 1,
		},
		{
			name: "no-signature skips signatures GET",
			args: []string{
				"mail", "+send",
				"--mailbox", "me",
				"--to", "alice@example.com",
				"--subject", "dry run",
				"--body", "<p>Hello</p>",
				"--no-signature",
				"--dry-run",
			},
		},
		{
			name: "explicit signature path shows signatures GET",
			args: []string{
				"mail", "+send",
				"--mailbox", "me",
				"--to", "alice@example.com",
				"--subject", "dry run",
				"--body", "<p>Hello</p>",
				"--signature-id", "sig_123",
				"--dry-run",
			},
			wantSignaturesGET:  true,
			wantSignatureIndex: 1,
		},
		{
			name: "plain-text default signature request is legal",
			args: []string{
				"mail", "+send",
				"--mailbox", "me",
				"--to", "alice@example.com",
				"--subject", "dry run",
				"--body", "Hello",
				"--plain-text",
				"--dry-run",
			},
			wantSignaturesGET:  true,
			wantSignatureIndex: 1,
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

			urls := gjson.Get(result.Stdout, "api.#.url").Array()
			hasSignatures := false
			for _, url := range urls {
				if url.String() == "/open-apis/mail/v1/user_mailboxes/me/settings/signatures" {
					hasSignatures = true
				}
			}
			assert.Equal(t, tt.wantSignaturesGET, hasSignatures, "stdout:\n%s", result.Stdout)
			if tt.wantSignaturesGET {
				idx := tt.wantSignatureIndex
				assert.Equal(t, "GET", gjson.Get(result.Stdout, "api."+string(rune('0'+idx))+".method").String(), "stdout:\n%s", result.Stdout)
				assert.Equal(t, "/open-apis/mail/v1/user_mailboxes/me/settings/signatures", gjson.Get(result.Stdout, "api."+string(rune('0'+idx))+".url").String(), "stdout:\n%s", result.Stdout)
			}
		})
	}
}

func TestMail_SendDryRunSignatureFlagValidation(t *testing.T) {
	setMailDraftSendDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"mail", "+send",
			"--to", "alice@example.com",
			"--subject", "dry run",
			"--body", "Hello",
			"--no-signature",
			"--signature-id", "sig_123",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	assert.Contains(t, result.Stdout+result.Stderr, "--no-signature and --signature-id are mutually exclusive")
}
