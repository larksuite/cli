// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func createWikiNode(t *testing.T, ctx context.Context, req clie2e.Request) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmdWithRetry(ctx, req, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	node := gjson.Get(result.Stdout, "data.node")
	require.True(t, node.Exists(), "stdout:\n%s", result.Stdout)

	return node
}

func findWikiNodeByToken(t *testing.T, ctx context.Context, spaceID string, nodeToken string) gjson.Result {
	t.Helper()

	require.NotEmpty(t, spaceID, "space ID is required")
	require.NotEmpty(t, nodeToken, "node token is required")

	pageToken := ""
	seenPageTokens := map[string]struct{}{}
	for {
		params := map[string]any{
			"space_id":  spaceID,
			"page_size": 50,
		}
		if pageToken != "" {
			if _, seen := seenPageTokens[pageToken]; seen {
				t.Fatalf("wiki node list pagination loop detected for space %q, repeated page_token %q", spaceID, pageToken)
			}
			seenPageTokens[pageToken] = struct{}{}
			params["page_token"] = pageToken
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"wiki", "nodes", "list"},
			DefaultAs: "bot",
			Params:    params,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		node := gjson.Get(result.Stdout, `data.items.#(node_token=="`+nodeToken+`")`)
		if node.Exists() {
			return node
		}

		hasMore := gjson.Get(result.Stdout, "data.has_more").Bool()
		pageToken = gjson.Get(result.Stdout, "data.page_token").String()
		if !hasMore || pageToken == "" {
			t.Fatalf("wiki node %q not found in listed pages, last stdout:\n%s", nodeToken, result.Stdout)
		}
	}
}
