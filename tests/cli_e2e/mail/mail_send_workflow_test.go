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

func TestMail_SendWorkflowAsUser(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	const mailboxID = "me"

	suffix := clie2e.GenerateSuffix()
	subject := "mail-self-" + suffix
	body := "self send body " + suffix
	replyBody := "self send reply body " + suffix
	forwardBody := "self send forward body " + suffix

	var primaryEmail string
	var sentMessageID string
	var threadID string
	var inboxMessageID string
	var replyDraftID string
	var forwardDraftID string

	parentT.Cleanup(func() {
		if replyDraftID != "" {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"mail", "user_mailbox.drafts", "delete"},
				DefaultAs: "user",
				Params: map[string]any{
					"user_mailbox_id": mailboxID,
					"draft_id":        replyDraftID,
				},
				Yes: true,
			})
			clie2e.ReportCleanupFailure(parentT, "delete reply draft "+replyDraftID, result, err)
		}

		if forwardDraftID != "" {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"mail", "user_mailbox.drafts", "delete"},
				DefaultAs: "user",
				Params: map[string]any{
					"user_mailbox_id": mailboxID,
					"draft_id":        forwardDraftID,
				},
				Yes: true,
			})
			clie2e.ReportCleanupFailure(parentT, "delete forward draft "+forwardDraftID, result, err)
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
		clie2e.ReportCleanupFailure(parentT, "trash self-send messages", result, err)
	})

	t.Run("get mailbox profile as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"mail", "user_mailboxes", "profile"},
			DefaultAs: "user",
			Params:    map[string]any{"user_mailbox_id": mailboxID},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		primaryEmail = gjson.Get(result.Stdout, "data.primary_email_address").String()
		require.NotEmpty(t, primaryEmail, "stdout:\n%s", result.Stdout)
	})

	t.Run("send mail to self with shortcut as user", func(t *testing.T) {
		require.NotEmpty(t, primaryEmail, "mailbox profile should be loaded before self-send")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+send",
				"--to", primaryEmail,
				"--subject", subject,
				"--body", body,
				"--plain-text",
				"--confirm-send",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		sentMessageID = gjson.Get(result.Stdout, "data.message_id").String()
		threadID = gjson.Get(result.Stdout, "data.thread_id").String()
		require.NotEmpty(t, sentMessageID, "stdout:\n%s", result.Stdout)
		require.NotEmpty(t, threadID, "stdout:\n%s", result.Stdout)
	})

	t.Run("find self sent mail in triage as user", func(t *testing.T) {
		require.NotEmpty(t, subject, "subject should be set before triage")

		var stdout string
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
			stdout = result.Stdout

			messages := gjson.Get(stdout, "messages").Array()
			for _, item := range messages {
				if item.Get("subject").String() != subject {
					continue
				}
				messageID := item.Get("message_id").String()
				if messageID == "" {
					continue
				}
				if messageID != sentMessageID {
					inboxMessageID = messageID
				}
			}

			if inboxMessageID != "" {
				require.GreaterOrEqual(t, int(gjson.Get(stdout, "count").Int()), 2, "stdout:\n%s", stdout)
				require.True(t, gjson.Get(stdout, `messages.#(message_id=="`+sentMessageID+`")`).Exists(), "stdout:\n%s", stdout)
				require.True(t, gjson.Get(stdout, `messages.#(message_id=="`+inboxMessageID+`")`).Exists(), "stdout:\n%s", stdout)
				return
			}

			time.Sleep(2 * time.Second)
		}

		t.Fatalf("failed to observe inbox copy for self-sent message in triage:\n%s", stdout)
	})

	t.Run("get sent message as user", func(t *testing.T) {
		require.NotEmpty(t, sentMessageID, "sent message id should be available before message read")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+message",
				"--mailbox", mailboxID,
				"--message-id", sentMessageID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, sentMessageID, gjson.Get(result.Stdout, "data.message_id").String())
		assert.Equal(t, threadID, gjson.Get(result.Stdout, "data.thread_id").String())
		assert.Equal(t, subject, gjson.Get(result.Stdout, "data.subject").String())
		assert.Equal(t, body, gjson.Get(result.Stdout, "data.body_plain_text").String())
		assert.Equal(t, "SENT", gjson.Get(result.Stdout, "data.folder_id").String())
		assert.Equal(t, "sent", gjson.Get(result.Stdout, "data.message_state_text").String())
		assert.Equal(t, primaryEmail, gjson.Get(result.Stdout, "data.to.0.mail_address").String())
	})

	t.Run("get received message as user", func(t *testing.T) {
		require.NotEmpty(t, inboxMessageID, "inbox message id should be available before message read")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+message",
				"--mailbox", mailboxID,
				"--message-id", inboxMessageID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, inboxMessageID, gjson.Get(result.Stdout, "data.message_id").String())
		assert.Equal(t, threadID, gjson.Get(result.Stdout, "data.thread_id").String())
		assert.Equal(t, subject, gjson.Get(result.Stdout, "data.subject").String())
		assert.Equal(t, body, gjson.Get(result.Stdout, "data.body_plain_text").String())
		assert.Equal(t, "INBOX", gjson.Get(result.Stdout, "data.folder_id").String())
		assert.Equal(t, "received", gjson.Get(result.Stdout, "data.message_state_text").String())
		assert.Equal(t, primaryEmail, gjson.Get(result.Stdout, "data.to.0.mail_address").String())
	})

	t.Run("get both self sent messages as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+messages",
				"--mailbox", mailboxID,
				"--message-ids", sentMessageID + "," + inboxMessageID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, int64(2), gjson.Get(result.Stdout, "data.total").Int())
		assert.Len(t, gjson.Get(result.Stdout, "data.unavailable_message_ids").Array(), 0, "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, `data.messages.#(message_id=="`+sentMessageID+`")`).Exists(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, `data.messages.#(message_id=="`+inboxMessageID+`")`).Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("get self send thread as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+thread",
				"--mailbox", mailboxID,
				"--thread-id", threadID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, threadID, gjson.Get(result.Stdout, "data.thread_id").String())
		assert.Equal(t, int64(1), gjson.Get(result.Stdout, "data.message_count").Int(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, `data.messages.#(message_id=="`+sentMessageID+`")`).Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("reply to received message with shortcut as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+reply",
				"--message-id", inboxMessageID,
				"--body", replyBody,
				"--plain-text",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		replyDraftID = gjson.Get(result.Stdout, "data.draft_id").String()
		require.NotEmpty(t, replyDraftID, "stdout:\n%s", result.Stdout)
	})

	t.Run("inspect reply draft as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+draft-edit",
				"--draft-id", replyDraftID,
				"--mailbox", mailboxID,
				"--inspect",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, "Re: "+subject, gjson.Get(result.Stdout, "data.projection.subject").String())
		assert.Equal(t, primaryEmail, gjson.Get(result.Stdout, "data.projection.to.0.address").String())
		assert.Contains(t, gjson.Get(result.Stdout, "data.projection.body_text").String(), replyBody)
		assert.Contains(t, gjson.Get(result.Stdout, "data.projection.body_text").String(), body)
		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.projection.in_reply_to").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("forward received message with shortcut as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+forward",
				"--message-id", inboxMessageID,
				"--to", primaryEmail,
				"--body", forwardBody,
				"--plain-text",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		forwardDraftID = gjson.Get(result.Stdout, "data.draft_id").String()
		require.NotEmpty(t, forwardDraftID, "stdout:\n%s", result.Stdout)
	})

	t.Run("inspect forward draft as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+draft-edit",
				"--draft-id", forwardDraftID,
				"--mailbox", mailboxID,
				"--inspect",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, "Fwd: "+subject, gjson.Get(result.Stdout, "data.projection.subject").String())
		assert.Equal(t, primaryEmail, gjson.Get(result.Stdout, "data.projection.to.0.address").String())
		assert.Contains(t, gjson.Get(result.Stdout, "data.projection.body_text").String(), forwardBody)
		assert.Contains(t, gjson.Get(result.Stdout, "data.projection.body_text").String(), body)
		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.projection.in_reply_to").String(), "stdout:\n%s", result.Stdout)
	})
}
