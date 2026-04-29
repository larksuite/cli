// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// sandboxUserOpenID is the sandbox evaluation account's open_id in the bot's tenant.
// The user may not be a member of the dynamically-created group chat, so the API may
// return a failed_user_reasons entry. Tests accept that partial-failure outcome and
// verify the structural response (feed_card_id, time_sensitive fields present).
const sandboxUserOpenID = "ou_34efadf32063b955134c1fbea3f08bad"

// TestFeedSensitive_EnableWorkflow proves the happy path for --enable:
// create a group chat (obtaining a real oc_ feed card ID), call
// feed +sensitive --enable, and assert the command returns exit 0 with a
// well-formed JSON envelope.
func TestFeedSensitive_EnableWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatName := "lark-cli-e2e-feed-sensitive-" + suffix

	// Dynamically create a group chat to obtain a real oc_ chat_id.
	// The chat_id serves as the feed_card_id for this test.
	chatID := createChatForFeed(t, parentT, ctx, chatName)

	t.Run("enable time-sensitive as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"feed", "+sensitive",
				"--feed-card-id", chatID,
				"--enable",
				"--user-ids", sandboxUserOpenID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		// Exit 0 on full success, 1 on partial failure (user not in chat) — both acceptable.
		assert.LessOrEqual(t, result.ExitCode, 1,
			"exit code should be 0 or 1, got: %d, stderr:\n%s", result.ExitCode, result.Stderr)

		stdout := strings.TrimSpace(result.Stdout)
		require.NotEmpty(t, stdout, "stdout should not be empty, stderr:\n%s", result.Stderr)

		// Spec output envelope: {"feed_card_id": "...", "time_sensitive": true, "failed_user_reasons": [...]}
		feedCardID := gjson.Get(stdout, "data.feed_card_id").String()
		assert.Equal(t, chatID, feedCardID,
			"feed_card_id in response must match the requested chat, stdout:\n%s", stdout)

		timeSensitive := gjson.Get(stdout, "data.time_sensitive")
		require.True(t, timeSensitive.Exists(),
			"time_sensitive field must be present in response, stdout:\n%s", stdout)
		assert.True(t, timeSensitive.Bool(),
			"time_sensitive must be true for --enable, stdout:\n%s", stdout)

		failedReasons := gjson.Get(stdout, "data.failed_user_reasons")
		if failedReasons.Exists() && len(failedReasons.Array()) > 0 {
			t.Logf("INFO: partial failure (user not in chat, expected): %s", failedReasons.Raw)
		}
	})
}

// TestFeedSensitive_DisableWorkflow proves the happy path for --disable:
// verifies that passing --disable results in time_sensitive=false in the
// response envelope.
func TestFeedSensitive_DisableWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatName := "lark-cli-e2e-feed-disable-" + suffix

	chatID := createChatForFeed(t, parentT, ctx, chatName)

	t.Run("disable time-sensitive as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"feed", "+sensitive",
				"--feed-card-id", chatID,
				"--disable",
				"--user-ids", sandboxUserOpenID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		// Exit 0 on full success, 1 on partial failure (user not in chat) — both acceptable.
		assert.LessOrEqual(t, result.ExitCode, 1,
			"exit code should be 0 or 1, got: %d, stderr:\n%s", result.ExitCode, result.Stderr)

		stdout := strings.TrimSpace(result.Stdout)
		require.NotEmpty(t, stdout, "stdout should not be empty, stderr:\n%s", result.Stderr)

		feedCardID := gjson.Get(stdout, "data.feed_card_id").String()
		assert.Equal(t, chatID, feedCardID,
			"feed_card_id in response must match the requested chat, stdout:\n%s", stdout)

		timeSensitive := gjson.Get(stdout, "data.time_sensitive")
		require.True(t, timeSensitive.Exists(),
			"time_sensitive field must be present in response, stdout:\n%s", stdout)
		assert.False(t, timeSensitive.Bool(),
			"time_sensitive must be false for --disable, stdout:\n%s", stdout)
	})
}

// TestFeedSensitive_DryRunFlagBehavior proves that --dry-run produces a PATCH
// request preview on stdout and exits 0 without making a real API call.
func TestFeedSensitive_DryRunFlagBehavior(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("dry-run prints PATCH preview", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"feed", "+sensitive",
				"--feed-card-id", "oc_dryruntestid",
				"--enable",
				"--user-ids", sandboxUserOpenID,
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		combined := result.Stdout + result.Stderr
		assert.Contains(t, combined, "PATCH",
			"dry-run output must contain HTTP method PATCH, combined:\n%s", combined)
		assert.Contains(t, combined, "/im/v2/feed_cards/",
			"dry-run output must contain the feed_cards API path, combined:\n%s", combined)
		assert.Contains(t, combined, "oc_dryruntestid",
			"dry-run output must contain the feed_card_id path segment, combined:\n%s", combined)
	})
}

// TestFeedSensitive_ValidationErrors proves the CLI validation layer:
//   - missing --enable/--disable → exit non-zero with a descriptive error
//   - both --enable and --disable together → exit non-zero (mutual exclusion)
//   - invalid feed-card-id (no oc_ prefix) → exit non-zero with validation error
//
// The spec says exit 1 for validation errors; the CLI framework may return exit 2
// for flag-parse failures. Both are accepted here as "not 0".
func TestFeedSensitive_ValidationErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("missing enable or disable flag", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"feed", "+sensitive",
				"--feed-card-id", "oc_testid",
				"--user-ids", sandboxUserOpenID,
				// intentionally omitting --enable and --disable
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode,
			"must fail when neither --enable nor --disable is provided, stdout:\n%s\nstderr:\n%s",
			result.Stdout, result.Stderr)

		combined := result.Stdout + result.Stderr
		assert.Contains(t, combined, "--enable",
			"error message must mention --enable flag, combined:\n%s", combined)
	})

	t.Run("enable and disable are mutually exclusive", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"feed", "+sensitive",
				"--feed-card-id", "oc_testid",
				"--enable",
				"--disable",
				"--user-ids", sandboxUserOpenID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode,
			"must fail when both --enable and --disable are provided, stdout:\n%s\nstderr:\n%s",
			result.Stdout, result.Stderr)

		combined := result.Stdout + result.Stderr
		assert.Contains(t, combined, "--disable",
			"error message must mention --disable flag, combined:\n%s", combined)
	})

	t.Run("invalid feed-card-id without oc_ prefix", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"feed", "+sensitive",
				"--feed-card-id", "invalid_id_no_oc_prefix",
				"--enable",
				"--user-ids", sandboxUserOpenID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode,
			"must fail for feed-card-id that does not start with oc_, stdout:\n%s\nstderr:\n%s",
			result.Stdout, result.Stderr)

		combined := result.Stdout + result.Stderr
		assert.Contains(t, combined, "oc_",
			"error message must mention the oc_ prefix requirement, combined:\n%s", combined)
	})
}

// createChatForFeed creates a private group chat via im +chat-create and
// returns the chat_id (which doubles as the feed_card_id for feed tests).
// No cleanup is registered because lark-cli im has no chat-delete command.
func createChatForFeed(t *testing.T, parentT *testing.T, ctx context.Context, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+chat-create",
			"--name", name,
			"--type", "private",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	chatID := gjson.Get(result.Stdout, "data.chat_id").String()
	require.NotEmpty(t, chatID, "chat_id must not be empty, stdout:\n%s", result.Stdout)

	// No chat-delete command exists in lark-cli im; chats are left in the account.
	_ = parentT

	return chatID
}
