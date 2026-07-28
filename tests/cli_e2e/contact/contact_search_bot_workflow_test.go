// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestContactSearchBotWorkflowAsUser(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"contact", "+search-bot", "--query", "助", "--format", "json"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	bots := gjson.Get(result.Stdout, "data.bots")
	require.True(t, bots.IsArray(), "data.bots must be an array; stdout:\n%s", result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "data.has_more").Exists(), "data.has_more must be present; stdout:\n%s", result.Stdout)
	for _, bot := range bots.Array() {
		require.NotEmpty(t, bot.Get("open_id").String(), "every bot must carry open_id; stdout:\n%s", result.Stdout)
	}
}
