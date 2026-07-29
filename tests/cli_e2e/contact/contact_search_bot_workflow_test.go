// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestContactSearchBotWorkflowAsUser proves the live round-trip without assuming
// anything about the tenant's bot inventory. An earlier version required at least
// one match for a hard-coded keyword, which is the tenant dependency that kept
// +search-user out of live coverage (see coverage.md): a tenant with no bot
// matching that word would fail the suite for no reason of ours.
//
// What is tenant-independent and still worth pinning: the command authenticates,
// the server accepts the request, and the envelope keeps its shape. The field
// assertions run over whatever rows came back, so zero rows is a pass.
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
	require.True(t, bots.IsArray(), "data.bots must be an array even when empty; stdout:\n%s", result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "data.has_more").Exists(), "data.has_more must be present; stdout:\n%s", result.Stdout)

	for _, bot := range bots.Array() {
		openID := bot.Get("open_id").String()
		require.NotEmpty(t, openID, "every bot must carry open_id; stdout:\n%s", result.Stdout)
		require.True(t, strings.HasPrefix(openID, "ou_"),
			"bot ids are open_ids; stdout:\n%s", result.Stdout)
		require.True(t, bot.Get("p2p_chat_id").Exists(),
			"p2p_chat_id must be present even when empty; stdout:\n%s", result.Stdout)
		require.True(t, bot.Get("match_segments").IsArray(),
			"match_segments must be an array, never null; stdout:\n%s", result.Stdout)
	}
}

// A filter without a keyword is rejected locally, so this costs no API call and
// holds in any tenant: it pins the contract that neither filter can enumerate.
func TestContactSearchBotRejectsFilterOnlyAsUser(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"contact", "+search-bot", "--has-chatted", "--format", "json"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	require.NotEqual(t, 0, result.ExitCode, "a filter-only request must not succeed; stderr:\n%s", result.Stderr)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)

	var named []string
	for _, p := range gjson.Get(result.Stderr, "error.params").Array() {
		named = append(named, p.Get("name").String())
	}
	require.ElementsMatch(t, []string{"--query", "--queries"}, named,
		"the error must name both ways to supply a keyword; stderr:\n%s", result.Stderr)
}
