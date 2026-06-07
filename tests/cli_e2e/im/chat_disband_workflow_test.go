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

func TestIM_ChatDisbandWorkflowAsBot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatName := "lark-cli-e2e-im-disband-" + suffix

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+chat-create",
			"--name", chatName,
			"--type", "private",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	chatID := gjson.Get(result.Stdout, "data.chat_id").String()
	require.NotEmpty(t, chatID, "chat_id should not be empty")

	disbandResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"im", "+chat-disband", "--chat-id", chatID},
		DefaultAs: "bot",
		Yes:       true,
	})
	require.NoError(t, err)
	if disbandResult.ExitCode != 0 && strings.Contains(disbandResult.Stderr, "required scope") {
		t.Skipf("skip chat disband workflow because the E2E app lacks im:chat:delete: %s", disbandResult.Stderr)
	}
	disbandResult.AssertExitCode(t, 0)
	disbandResult.AssertStdoutStatus(t, true)
	require.Equal(t, chatID, gjson.Get(disbandResult.Stdout, "data.chat_id").String(), "stdout:\n%s", disbandResult.Stdout)
	require.True(t, gjson.Get(disbandResult.Stdout, "data.disbanded").Bool(), "stdout:\n%s", disbandResult.Stdout)
}
