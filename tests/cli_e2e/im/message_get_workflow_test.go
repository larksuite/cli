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

func TestIM_MessageGetWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	chatName := "im-lookup-" + suffix
	messageText := "im-msg-" + suffix

	chatID := createChatAs(t, parentT, ctx, chatName, "user")
	messageID := sendMessageAs(t, ctx, chatID, messageText, "user")

	t.Run("batch get message as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"im", "+messages-mget", "--message-ids", messageID},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		messages := gjson.Get(result.Stdout, "data.messages").Array()
		require.Len(t, messages, 1, "stdout:\n%s", result.Stdout)
		require.Equal(t, messageID, messages[0].Get("message_id").String(), "stdout:\n%s", result.Stdout)
		require.True(t, strings.Contains(messages[0].Get("content").String(), messageText), "stdout:\n%s", result.Stdout)
	})
}
