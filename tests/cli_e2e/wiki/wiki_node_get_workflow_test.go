// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWiki_NodeGetWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	spaceID := getWikiSpace(t, ctx, "my_library").Get("space_id").String()
	require.NotEmpty(t, spaceID)
	title := "lark-cli-e2e-node-get-" + clie2e.GenerateSuffix()
	node, created, err := createWikiNode(t, t, ctx, spaceID, map[string]any{
		"node_type": "origin", "obj_type": "docx", "title": title,
	})
	require.NoError(t, err)
	created.AssertExitCode(t, 0)
	nodeToken, objToken := node.Get("node_token").String(), node.Get("obj_token").String()
	require.NotEmpty(t, nodeToken)
	require.NotEmpty(t, objToken)

	for _, tt := range []struct{ name, input string }{
		{"wiki token", nodeToken},
		{"document token without type", objToken},
		{"wiki URL", "https://example.com/wiki/" + nodeToken},
		{"document URL", "https://example.com/docx/" + objToken},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"wiki", "+node-get", "--node-token", tt.input, "--space-id", spaceID},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			require.Empty(t, result.Stderr)
			data := gjson.Get(result.Stdout, "data")
			require.Equal(t, nodeToken, data.Get("node_token").String())
			require.Equal(t, objToken, data.Get("obj_token").String())
			require.Equal(t, "docx", data.Get("obj_type").String())
			require.Equal(t, title, data.Get("title").String())
			require.Equal(t, spaceID, data.Get("space_id").String())
			require.False(t, data.Get("url").Exists())
		})
	}

	t.Run("legacy object type is silently ignored", func(t *testing.T) {
		for _, legacyType := range []string{"docx", "sheet", "unknown", ""} {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"wiki", "+node-get", "--node-token", "https://example.com/docx/" + objToken, "--obj-type=" + legacyType},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			require.Empty(t, result.Stderr)
			require.Equal(t, nodeToken, gjson.Get(result.Stdout, "data.node_token").String())
			require.Equal(t, "docx", gjson.Get(result.Stdout, "data.obj_type").String())
		}
	})

	t.Run("server rejects short token", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"wiki", "+node-get", "--node-token", "short", "--obj-type=unknown"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 1)
		require.Empty(t, result.Stdout)
		require.True(t, gjson.Valid(result.Stderr), "stderr must contain only the JSON error envelope: %s", result.Stderr)
		require.Equal(t, int64(131016), gjson.Get(result.Stderr, "error.code").Int())
		require.Equal(t, "invalid_parameters", gjson.Get(result.Stderr, "error.subtype").String())
		require.False(t, gjson.Get(result.Stderr, "error.retryable").Bool())
		require.NotEmpty(t, gjson.Get(result.Stderr, "error.log_id").String())
	})
}
