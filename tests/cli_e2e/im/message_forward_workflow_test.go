// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIM_MessageForwardWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	suffix := clie2e.GenerateSuffix()
	messageText := "im-forward-msg-" + suffix
	replyText := "im-forward-reply-" + suffix

	selfOpenID := getSelfOpenID(t, ctx)
	chatID, messageID := sendDirectMessageToUser(t, ctx, selfOpenID, messageText, "bot")

	t.Run("forward message with api command as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"im", "messages", "forward"},
			DefaultAs: "user",
			Params: map[string]any{
				"message_id":      messageID,
				"receive_id_type": "chat_id",
				"uuid":            "msg-forward-" + suffix,
			},
			Data: map[string]any{
				"receive_id": chatID,
			},
		})
		require.NoError(t, err)
		skipIfMissingIMForwardPermission(t, result)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		forwardedID := gjson.Get(result.Stdout, "data.message_id").String()
		require.NotEmpty(t, forwardedID, "stdout:\n%s", result.Stdout)
		require.NotEqual(t, messageID, forwardedID, "stdout:\n%s", result.Stdout)
		require.Equal(t, chatID, gjson.Get(result.Stdout, "data.chat_id").String(), "stdout:\n%s", result.Stdout)
	})

	var threadID string
	t.Run("create thread fixture as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-reply",
				"--message-id", messageID,
				"--text", replyText,
				"--reply-in-thread",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		threadID = findThreadIDForMessage(t, ctx, chatID, messageID, "bot")
	})

	t.Run("forward thread with api command as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"im", "threads", "forward"},
			DefaultAs: "user",
			Params: map[string]any{
				"thread_id":       threadID,
				"receive_id_type": "chat_id",
				"uuid":            "thread-forward-" + suffix,
			},
			Data: map[string]any{
				"receive_id": chatID,
			},
		})
		require.NoError(t, err)
		skipIfMissingIMForwardPermission(t, result)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		forwardedID := gjson.Get(result.Stdout, "data.message_id").String()
		require.NotEmpty(t, forwardedID, "stdout:\n%s", result.Stdout)
		require.Equal(t, chatID, gjson.Get(result.Stdout, "data.chat_id").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, "merge_forward", gjson.Get(result.Stdout, "data.msg_type").String(), "stdout:\n%s", result.Stdout)
	})
}

func findThreadIDForMessage(t *testing.T, ctx context.Context, chatID string, messageID string, defaultAs string) string {
	t.Helper()

	listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"im", "+chat-messages-list",
			"--chat-id", chatID,
			"--start", time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
			"--end", time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
		},
		DefaultAs: defaultAs,
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			if result == nil || result.ExitCode != 0 {
				return true
			}
			for _, item := range gjson.Get(result.Stdout, "data.messages").Array() {
				if item.Get("message_id").String() == messageID && item.Get("thread_id").String() != "" {
					return false
				}
			}
			return true
		},
	})
	require.NoError(t, err)
	listResult.AssertExitCode(t, 0)
	listResult.AssertStdoutStatus(t, true)

	for _, item := range gjson.Get(listResult.Stdout, "data.messages").Array() {
		if item.Get("message_id").String() == messageID {
			threadID := item.Get("thread_id").String()
			require.NotEmpty(t, threadID, "expected thread_id for message %s in stdout:\n%s", messageID, listResult.Stdout)
			return threadID
		}
	}

	t.Fatalf("expected message %s in stdout:\n%s", messageID, listResult.Stdout)
	return ""
}

func getSelfOpenID(t *testing.T, ctx context.Context) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"contact", "+get-user"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	openID := gjson.Get(result.Stdout, "data.user.open_id").String()
	require.NotEmpty(t, openID, "stdout:\n%s", result.Stdout)
	return openID
}

func sendDirectMessageToUser(t *testing.T, ctx context.Context, userOpenID string, text string, defaultAs string) (string, string) {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-send",
			"--user-id", userOpenID,
			"--text", text,
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	chatID := gjson.Get(result.Stdout, "data.chat_id").String()
	messageID := gjson.Get(result.Stdout, "data.message_id").String()
	require.NotEmpty(t, chatID, "stdout:\n%s", result.Stdout)
	require.NotEmpty(t, messageID, "stdout:\n%s", result.Stdout)
	return chatID, messageID
}

func skipIfMissingIMForwardPermission(t *testing.T, result *clie2e.Result) {
	t.Helper()
	if result == nil || result.ExitCode == 0 {
		return
	}
	stderrLower := strings.ToLower(result.Stderr)
	if strings.Contains(stderrLower, "permission denied") ||
		strings.Contains(stderrLower, "230027") ||
		strings.Contains(stderrLower, "missing_scope") {
		t.Skipf("skip UAT forward workflow due to missing IM forward permissions: %s", result.Stderr)
	}
}
