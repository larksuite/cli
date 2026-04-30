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

func createWikiNode(t *testing.T, parentT *testing.T, ctx context.Context, spaceID string, data map[string]any) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "post", "/open-apis/wiki/v2/spaces/" + spaceID + "/nodes"},
		DefaultAs: "bot",
		Data:      data,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	node := gjson.Get(result.Stdout, "data.node")
	require.True(t, node.Exists(), "stdout:\n%s", result.Stdout)

	nodeToken := node.Get("node_token").String()
	require.NotEmpty(t, nodeToken, "stdout:\n%s", result.Stdout)
	objType := node.Get("obj_type").String()
	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := deleteWikiNode(cleanupCtx, spaceID, nodeToken, objType)
		clie2e.ReportCleanupFailure(parentT, "delete wiki node "+nodeToken, deleteResult, deleteErr)
	})

	return node
}

func getWikiNode(t *testing.T, ctx context.Context, nodeToken string) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/get_node"},
		DefaultAs: "bot",
		Params:    map[string]any{"token": nodeToken},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	node := gjson.Get(result.Stdout, "data.node")
	require.True(t, node.Exists(), "stdout:\n%s", result.Stdout)
	return node
}

func getWikiSpace(t *testing.T, ctx context.Context, spaceID string) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/" + spaceID},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	space := gjson.Get(result.Stdout, "data.space")
	require.True(t, space.Exists(), "stdout:\n%s", result.Stdout)
	return space
}

func listWikiSpaces(t *testing.T, ctx context.Context, pageSize int) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces"},
		DefaultAs: "bot",
		Params:    map[string]any{"page_size": pageSize},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)
	return gjson.Parse(result.Stdout)
}

// deleteWikiNode removes a wiki space node. The DELETE endpoint requires
// obj_type as a body field (validation error 99992402 if omitted), so
// pass it via --data rather than --params even though DELETE bodies are
// uncommon.
func deleteWikiNode(ctx context.Context, spaceID, nodeToken, objType string) (*clie2e.Result, error) {
	return clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "delete", "/open-apis/wiki/v2/spaces/" + spaceID + "/nodes/" + nodeToken},
		DefaultAs: "bot",
		Data:      map[string]any{"obj_type": objType},
	})
}

func findWikiNodeByToken(t *testing.T, ctx context.Context, spaceID string, nodeToken string) gjson.Result {
	t.Helper()

	pageToken := ""
	lastStdout := ""
	seenPageTokens := map[string]struct{}{}
	for {
		params := map[string]any{"page_size": 50}
		if pageToken != "" {
			if _, exists := seenPageTokens[pageToken]; exists {
				t.Fatalf("wiki list pagination loop detected for page_token %q, last stdout:\n%s", pageToken, lastStdout)
			}
			seenPageTokens[pageToken] = struct{}{}
			params["page_token"] = pageToken
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/" + spaceID + "/nodes"},
			DefaultAs: "bot",
			Params:    params,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		lastStdout = result.Stdout
		parsed := gjson.Parse(result.Stdout)
		node := parsed.Get(`data.items.#(node_token=="` + nodeToken + `")`)
		if node.Exists() {
			return node
		}

		pageToken = parsed.Get("data.page_token").String()
		if pageToken == "" || !parsed.Get("data.has_more").Bool() {
			t.Fatalf("wiki node %q not found in listed pages, last stdout:\n%s", nodeToken, lastStdout)
		}
	}
}
