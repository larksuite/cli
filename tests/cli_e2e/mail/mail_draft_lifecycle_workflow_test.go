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

func TestMail_DraftLifecycleWorkflowAsUser(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	suffix := clie2e.GenerateSuffix()
	originalSubject := "lark-cli-e2e-mail-draft-" + suffix
	updatedSubject := originalSubject + "-updated"
	originalBody := "draft lifecycle body " + suffix

	const mailboxID = "me"

	var draftID string
	var draftDeleted bool

	parentT.Cleanup(func() {
		if draftID == "" || draftDeleted {
			return
		}

		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"mail", "user_mailbox.drafts", "delete"},
			DefaultAs: "user",
			Params: map[string]any{
				"user_mailbox_id": mailboxID,
				"draft_id":        draftID,
			},
		})
		clie2e.ReportCleanupFailure(parentT, "delete draft "+draftID, result, err)
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

		require.NotEmpty(t, gjson.Get(result.Stdout, "data.primary_email_address").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("create draft with shortcut as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+draft-create",
				"--subject", originalSubject,
				"--body", originalBody,
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

	t.Run("list draft as user", func(t *testing.T) {
		require.NotEmpty(t, draftID, "draft should be created before listing drafts")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"mail", "user_mailbox.drafts", "list"},
			DefaultAs: "user",
			Params: map[string]any{
				"user_mailbox_id": mailboxID,
				"page_size":       100,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		require.True(t, gjson.Get(result.Stdout, `data.items.#(id=="`+draftID+`")`).Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("get created draft as user", func(t *testing.T) {
		require.NotEmpty(t, draftID, "draft should be created before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"mail", "user_mailbox.drafts", "get"},
			DefaultAs: "user",
			Params: map[string]any{
				"user_mailbox_id": mailboxID,
				"draft_id":        draftID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, draftID, gjson.Get(result.Stdout, "data.draft.id").String())
		assert.Equal(t, originalSubject, gjson.Get(result.Stdout, "data.draft.message.subject").String())
		assert.Equal(t, int64(3), gjson.Get(result.Stdout, "data.draft.message.message_state").Int())
	})

	t.Run("inspect created draft as user", func(t *testing.T) {
		require.NotEmpty(t, draftID, "draft should be created before inspect")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+draft-edit",
				"--draft-id", draftID,
				"--mailbox", mailboxID,
				"--inspect",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, draftID, gjson.Get(result.Stdout, "data.draft_id").String())
		assert.Equal(t, originalSubject, gjson.Get(result.Stdout, "data.projection.subject").String())
		assert.Equal(t, originalBody, gjson.Get(result.Stdout, "data.projection.body_text").String())
	})

	t.Run("update draft subject with shortcut as user", func(t *testing.T) {
		require.NotEmpty(t, draftID, "draft should be created before update")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+draft-edit",
				"--draft-id", draftID,
				"--mailbox", mailboxID,
				"--set-subject", updatedSubject,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, draftID, gjson.Get(result.Stdout, "data.draft_id").String())
		assert.Equal(t, updatedSubject, gjson.Get(result.Stdout, "data.projection.subject").String())
		assert.Equal(t, originalBody, gjson.Get(result.Stdout, "data.projection.body_text").String())
	})

	t.Run("inspect updated draft as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"mail", "+draft-edit",
				"--draft-id", draftID,
				"--mailbox", mailboxID,
				"--inspect",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.Equal(t, updatedSubject, gjson.Get(result.Stdout, "data.projection.subject").String())
		assert.Equal(t, originalBody, gjson.Get(result.Stdout, "data.projection.body_text").String())
	})

	t.Run("delete draft as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"mail", "user_mailbox.drafts", "delete"},
			DefaultAs: "user",
			Params: map[string]any{
				"user_mailbox_id": mailboxID,
				"draft_id":        draftID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		draftDeleted = true
	})

	t.Run("verify draft removed from list as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"mail", "user_mailbox.drafts", "get"},
			DefaultAs: "user",
			Params: map[string]any{
				"user_mailbox_id": mailboxID,
				"draft_id":        draftID,
			},
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
		assert.Equal(t, "not_found", gjson.Get(result.Stderr, "error.detail.type").String(), "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	})
}
