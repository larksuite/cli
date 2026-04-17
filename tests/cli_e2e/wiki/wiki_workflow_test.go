// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWiki_NodeWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	parentTitle := "lark-cli-e2e-wiki-parent-" + suffix
	createdTitle := "lark-cli-e2e-wiki-create-" + suffix
	copiedTitle := "lark-cli-e2e-wiki-copy-" + suffix

	var spaceID string
	var parentNodeToken string
	var createdNodeToken string
	var createdObjToken string
	var copiedNodeToken string
	var copiedSpaceID string

	t.Run("create isolated parent node as bot", func(t *testing.T) {
		parentNode := createWikiNode(t, parentT, ctx, "my_library", map[string]any{
			"node_type": "origin",
			"obj_type":  "docx",
			"title":     parentTitle,
		})
		spaceID = parentNode.Get("space_id").String()
		parentNodeToken = parentNode.Get("node_token").String()
		require.NotEmpty(t, spaceID)
		require.NotEmpty(t, parentNodeToken)
		assert.Equal(t, parentTitle, parentNode.Get("title").String())
	})

	t.Run("create node as bot", func(t *testing.T) {
		require.NotEmpty(t, parentNodeToken, "parent node token should be created before child node")

		node := createWikiNode(t, parentT, ctx, spaceID, map[string]any{
			"node_type":         "origin",
			"obj_type":          "docx",
			"title":             createdTitle,
			"parent_node_token": parentNodeToken,
		})

		createdNodeToken = node.Get("node_token").String()
		createdObjToken = node.Get("obj_token").String()
		require.NotEmpty(t, createdNodeToken)
		require.NotEmpty(t, createdObjToken)
		assert.Equal(t, createdTitle, node.Get("title").String())
		assert.Equal(t, "origin", node.Get("node_type").String())
		assert.Equal(t, "docx", node.Get("obj_type").String())
		assert.Equal(t, parentNodeToken, node.Get("parent_node_token").String())
	})

	t.Run("get created node as bot", func(t *testing.T) {
		require.NotEmpty(t, createdNodeToken, "node token should be created before get_node")
		node := getWikiNode(t, ctx, createdNodeToken)
		assert.Equal(t, createdNodeToken, node.Get("node_token").String())
		assert.Equal(t, createdObjToken, node.Get("obj_token").String())
		assert.Equal(t, createdTitle, node.Get("title").String())
		assert.Equal(t, spaceID, node.Get("space_id").String())
	})

	t.Run("get isolated parent node as bot", func(t *testing.T) {
		require.NotEmpty(t, parentNodeToken, "parent node token should be created before get_node")
		node := getWikiNode(t, ctx, parentNodeToken)
		assert.Equal(t, parentNodeToken, node.Get("node_token").String())
		assert.Equal(t, parentTitle, node.Get("title").String())
	})

	t.Run("get space as bot", func(t *testing.T) {
		require.NotEmpty(t, spaceID, "space ID should be available before get")
		space := getWikiSpace(t, ctx, spaceID)
		assert.Equal(t, spaceID, space.Get("space_id").String())
		assert.NotEmpty(t, space.Get("name").String())
	})

	t.Run("list spaces as bot", func(t *testing.T) {
		result := listWikiSpaces(t, ctx, 1)
		assert.True(t, result.Get("data.items").Exists(), "stdout:\n%s", result.Raw)
	})

	t.Run("list nodes and find isolated parent node as bot", func(t *testing.T) {
		require.NotEmpty(t, spaceID, "space ID should be available before list")
		require.NotEmpty(t, parentNodeToken, "parent node token should be available before list")

		nodeItem := findWikiNodeByToken(t, ctx, spaceID, parentNodeToken)
		assert.Equal(t, parentTitle, nodeItem.Get("title").String())
	})

	t.Run("copy node as bot", func(t *testing.T) {
		require.NotEmpty(t, spaceID, "space ID should be available before copy")
		require.NotEmpty(t, createdNodeToken, "node token should be available before copy")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"api", "post", "/open-apis/wiki/v2/spaces/" + spaceID + "/nodes/" + createdNodeToken + "/copy"},
			DefaultAs: "bot",
			Data: map[string]any{
				"target_space_id": spaceID,
				"title":           copiedTitle,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		copiedNodeToken = gjson.Get(result.Stdout, "data.node.node_token").String()
		copiedSpaceID = gjson.Get(result.Stdout, "data.node.space_id").String()
		require.NotEmpty(t, copiedNodeToken)
		require.NotEmpty(t, copiedSpaceID)
		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

			deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"api", "delete", "/open-apis/wiki/v2/spaces/" + copiedSpaceID + "/nodes/" + copiedNodeToken},
				DefaultAs: "bot",
			})
			clie2e.ReportCleanupFailure(parentT, "delete copied wiki node "+copiedNodeToken, deleteResult, deleteErr)
		})
	})

	t.Run("get copied node as bot", func(t *testing.T) {
		require.NotEmpty(t, copiedNodeToken, "copied node token should be available before verification")
		node := getWikiNode(t, ctx, copiedNodeToken)
		assert.Equal(t, copiedTitle, node.Get("title").String())
	})
}
