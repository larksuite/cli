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

func TestIM_MessageUpdateWorkflowAsBot(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatName := "lark-cli-e2e-im-update-" + suffix
	originalText := "lark-cli-e2e-update-original-" + suffix
	updatedText := "lark-cli-e2e-update-edited-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendMessage(t, ctx, chatID, originalText)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-update",
			"--message-id", messageID,
			"--text", updatedText,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, messageID, gjson.Get(result.Stdout, "data.message_id").String(), "stdout:\n%s", result.Stdout)
	require.Equal(t, chatID, gjson.Get(result.Stdout, "data.chat_id").String(), "stdout:\n%s", result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "data.updated").Bool(), "stdout:\n%s", result.Stdout)

	getResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{"im", "+messages-mget",
			"--message-ids", messageID,
			"--no-reactions",
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			if result == nil || result.ExitCode != 0 {
				return true
			}
			messages := gjson.Get(result.Stdout, "data.messages").Array()
			if len(messages) != 1 {
				return true
			}
			return !strings.Contains(messages[0].Get("content").String(), updatedText)
		},
	})
	require.NoError(t, err)
	getResult.AssertExitCode(t, 0)
	getResult.AssertStdoutStatus(t, true)

	messages := gjson.Get(getResult.Stdout, "data.messages").Array()
	require.Len(t, messages, 1, "stdout:\n%s", getResult.Stdout)
	require.Equal(t, messageID, messages[0].Get("message_id").String(), "stdout:\n%s", getResult.Stdout)
	require.True(t, strings.Contains(messages[0].Get("content").String(), updatedText), "stdout:\n%s", getResult.Stdout)
	require.True(t, messages[0].Get("updated").Bool(), "stdout:\n%s", getResult.Stdout)
}

func TestIM_MessageCardUpdateWorkflowAsBot(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatName := "lark-cli-e2e-im-card-update-" + suffix
	originalText := "lark-cli-e2e-card-original-" + suffix
	updatedText := "lark-cli-e2e-card-updated-" + suffix

	chatID := createChat(t, parentT, ctx, chatName)
	messageID := sendInteractiveCard(t, ctx, chatID, originalText)
	updatedCard := simpleInteractiveCard(updatedText)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-card-update",
			"--message-id", messageID,
			"--content", updatedCard,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, messageID, gjson.Get(result.Stdout, "data.message_id").String(), "stdout:\n%s", result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "data.updated").Bool(), "stdout:\n%s", result.Stdout)

	getResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{"im", "+messages-mget",
			"--message-ids", messageID,
			"--no-reactions",
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			if result == nil || result.ExitCode != 0 {
				return true
			}
			messages := gjson.Get(result.Stdout, "data.messages").Array()
			if len(messages) != 1 {
				return true
			}
			return !strings.Contains(messages[0].Get("content").String(), updatedText)
		},
	})
	require.NoError(t, err)
	getResult.AssertExitCode(t, 0)
	getResult.AssertStdoutStatus(t, true)

	messages := gjson.Get(getResult.Stdout, "data.messages").Array()
	require.Len(t, messages, 1, "stdout:\n%s", getResult.Stdout)
	require.Equal(t, messageID, messages[0].Get("message_id").String(), "stdout:\n%s", getResult.Stdout)
	require.True(t, strings.Contains(messages[0].Get("content").String(), updatedText), "stdout:\n%s", getResult.Stdout)
}

func sendInteractiveCard(t *testing.T, ctx context.Context, chatID string, text string) string {
	t.Helper()
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-send",
			"--chat-id", chatID,
			"--msg-type", "interactive",
			"--content", simpleInteractiveCard(text),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	messageID := gjson.Get(result.Stdout, "data.message_id").String()
	require.NotEmpty(t, messageID, "message_id should not be empty")
	return messageID
}

func simpleInteractiveCard(text string) string {
	return `{"config":{"update_multi":true},"elements":[{"tag":"div","text":{"tag":"plain_text","content":"` + text + `"}}]}`
}
