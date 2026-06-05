// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"strconv"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMail_DraftSendDryRun(t *testing.T) {
	setMailDraftSendDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"mail", "+draft-send",
			"--mailbox", "alias@example.com",
			"--draft-id", " draft_001, draft_002 ",
			"--draft-id", " draft_003 ",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	wantURLs := []string{
		"/open-apis/mail/v1/user_mailboxes/alias@example.com/drafts/draft_001/send",
		"/open-apis/mail/v1/user_mailboxes/alias@example.com/drafts/draft_002/send",
		"/open-apis/mail/v1/user_mailboxes/alias@example.com/drafts/draft_003/send",
	}
	assert.Equal(t, int64(len(wantURLs)), gjson.Get(result.Stdout, "api.#").Int(), "stdout:\n%s", result.Stdout)
	for i, wantURL := range wantURLs {
		idx := strconv.Itoa(i)
		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api."+idx+".method").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, wantURL, gjson.Get(result.Stdout, "api."+idx+".url").String(), "stdout:\n%s", result.Stdout)
		assert.False(t, gjson.Get(result.Stdout, "api."+idx+".body").Exists(), "stdout:\n%s", result.Stdout)
	}
}

func TestMail_DraftSendDryRunValidation(t *testing.T) {
	setMailDraftSendDryRunEnv(t)

	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name: "reject whitespace draft id",
			args: []string{
				"mail", "+draft-send",
				"--draft-id", "   ",
				"--dry-run",
			},
			wantMsg: "--draft-id contains empty value",
		},
		{
			name: "reject too many draft ids",
			args: []string{
				"mail", "+draft-send",
				"--draft-id", manyDraftIDsForE2E(51),
				"--dry-run",
			},
			wantMsg: "too many drafts",
		},
		{
			name: "reject duplicate draft id",
			args: []string{
				"mail", "+draft-send",
				"--draft-id", "draft_001,draft_002,draft_001",
				"--dry-run",
			},
			wantMsg: "--draft-id contains duplicate value: draft_001",
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
			result.AssertExitCode(t, 2)
			output := result.Stdout + result.Stderr
			assert.Contains(t, output, tt.wantMsg, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
		})
	}
}

func setMailDraftSendDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "mail_draft_send_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "mail_draft_send_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func manyDraftIDsForE2E(n int) string {
	ids := make([]byte, 0, n*4)
	for i := 0; i < n; i++ {
		if i > 0 {
			ids = append(ids, ',')
		}
		ids = append(ids, 'd')
		ids = strconv.AppendInt(ids, int64(i), 10)
	}
	return string(ids)
}
