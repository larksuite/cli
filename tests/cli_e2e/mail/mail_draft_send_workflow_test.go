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

func TestMail_DraftSendWorkflowAsUser(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	const mailboxID = "me"
	suffix := clie2e.GenerateSuffix()
	subject := "lark-cli-e2e-mail-draft-send-" + suffix
	body := "draft-send workflow body " + suffix

	var primaryEmail string
	var draftID string
	var draftSent bool
	var sentMessageID string
	var inboxMessageID string

	parentT.Cleanup(func() {
		if draftID != "" && !draftSent {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"mail", "user_mailbox.drafts", "delete"},
				DefaultAs: "user",
				Params: map[string]any{
					"user_mailbox_id": mailboxID,
					"draft_id":        draftID,
				},
				Yes: true,
			})
			clie2e.ReportCleanupFailure(parentT, "delete draft "+draftID, result, err)
		}

		var messageIDs []string
		if sentMessageID != "" {
			messageIDs = append(messageIDs, sentMessageID)
		}
		if inboxMessageID != "" && inboxMessageID != sentMessageID {
			messageIDs = append(messageIDs, inboxMessageID)
		}
		if len(messageIDs) == 0 {
			return
		}

		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"mail", "user_mailbox.messages", "batch_trash"},
			DefaultAs: "user",
			Params:    map[string]any{"user_mailbox_id": mailboxID},
			Data:      map[string]any{"message_ids": messageIDs},
		})
		clie2e.ReportCleanupFailure(parentT, "trash draft-send messages", result, err)
	})

	t.Run("get mailbox profile as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"mail", "user_mailboxes", "profile"},
			DefaultAs: "user",
			Params:    map[string]any{"user_mailbox_id": mailboxID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		primaryEmail = gjson.Get(result.Stdout, "data.primary_email_address").String()
		require.NotEmpty(t, primaryEmail, "stdout:\n%s", result.Stdout)
	})

	t.Run("create self-addressed draft as user", func(t *testing.T) {
		require.NotEmpty(t, primaryEmail, "mailbox profile should be loaded before draft create")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+draft-create",
				"--to", primaryEmail,
				"--subject", subject,
				"--body", body,
				"--plain-text",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		draftID = gjson.Get(result.Stdout, "data.draft_id").String()
		require.NotEmpty(t, draftID, "stdout:\n%s", result.Stdout)
	})

	t.Run("send draft with shortcut as user", func(t *testing.T) {
		require.NotEmpty(t, draftID, "draft should be created before +draft-send")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+draft-send",
				"--mailbox", mailboxID,
				"--draft-id", draftID,
			},
			DefaultAs: "user",
			Yes:       true,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, int64(1), gjson.Get(result.Stdout, "data.total").Int(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, int64(1), gjson.Get(result.Stdout, "data.success_count").Int(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, int64(0), gjson.Get(result.Stdout, "data.failure_count").Int(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, draftID, gjson.Get(result.Stdout, "data.sent.0.draft_id").String(), "stdout:\n%s", result.Stdout)

		sentMessageID = gjson.Get(result.Stdout, "data.sent.0.message_id").String()
		require.NotEmpty(t, sentMessageID, "stdout:\n%s", result.Stdout)
		draftSent = true
	})

	t.Run("find self-received message for cleanup", func(t *testing.T) {
		require.NotEmpty(t, sentMessageID, "draft should be sent before triage lookup")

		for attempt := 0; attempt < 12; attempt++ {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"mail", "+triage",
					"--mailbox", mailboxID,
					"--query", subject,
					"--max", "10",
					"--format", "data",
				},
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			for _, item := range gjson.Get(result.Stdout, "messages").Array() {
				if item.Get("subject").String() != subject {
					continue
				}
				messageID := item.Get("message_id").String()
				if messageID != "" && messageID != sentMessageID {
					inboxMessageID = messageID
					return
				}
			}
			time.Sleep(2 * time.Second)
		}
	})
}
