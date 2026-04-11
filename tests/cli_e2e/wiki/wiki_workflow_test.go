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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	createdTitle := "lark-cli-e2e-wiki-create-" + suffix
	copiedTitle := "lark-cli-e2e-wiki-copy-" + suffix

	var spaceID string
	var createdNodeToken string
	var createdObjToken string
	var copiedNodeToken string

	t.Run("create node", func(t *testing.T) {
		node := createWikiNode(t, ctx, clie2e.Request{
			Args:      []string{"wiki", "nodes", "create"},
			DefaultAs: "bot",
			Params: map[string]any{
				"space_id": "my_library",
			},
			Data: map[string]any{
				"node_type": "origin",
				"obj_type":  "docx",
				"title":     createdTitle,
			},
		})

		spaceID = node.Get("space_id").String()
		createdNodeToken = node.Get("node_token").String()
		createdObjToken = node.Get("obj_token").String()
		require.NotEmpty(t, spaceID)
		require.NotEmpty(t, createdNodeToken)
		require.NotEmpty(t, createdObjToken)
		assert.Equal(t, createdTitle, node.Get("title").String())
		assert.Equal(t, "origin", node.Get("node_type").String())
		assert.Equal(t, "docx", node.Get("obj_type").String())
	})

	t.Run("get created node", func(t *testing.T) {
		require.NotEmpty(t, createdNodeToken, "node token should be created before get_node")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"wiki", "spaces", "get_node"},
			DefaultAs: "bot",
			Params: map[string]any{
				"token":    createdNodeToken,
				"obj_type": "wiki",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, createdNodeToken, gjson.Get(result.Stdout, "data.node.node_token").String())
		assert.Equal(t, createdObjToken, gjson.Get(result.Stdout, "data.node.obj_token").String())
		assert.Equal(t, createdTitle, gjson.Get(result.Stdout, "data.node.title").String())
		assert.Equal(t, spaceID, gjson.Get(result.Stdout, "data.node.space_id").String())
	})

	t.Run("get space", func(t *testing.T) {
		require.NotEmpty(t, spaceID, "space ID should be available before get")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"wiki", "spaces", "get"},
			DefaultAs: "bot",
			Params: map[string]any{
				"space_id": spaceID,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.Equal(t, spaceID, gjson.Get(result.Stdout, "data.space.space_id").String())
		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.space.name").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("list spaces", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"wiki", "spaces", "list"},
			DefaultAs: "bot",
			Params: map[string]any{
				"page_size": 1,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		assert.True(t, gjson.Get(result.Stdout, "data.page_token").Exists(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, "data.items").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("list nodes and find created node", func(t *testing.T) {
		require.NotEmpty(t, spaceID, "space ID should be available before list")
		require.NotEmpty(t, createdNodeToken, "node token should be available before list")

		nodeItem := findWikiNodeByToken(t, ctx, spaceID, createdNodeToken)
		assert.Equal(t, createdTitle, nodeItem.Get("title").String())
		assert.Equal(t, createdObjToken, nodeItem.Get("obj_token").String())
	})

	t.Run("copy node", func(t *testing.T) {
		require.NotEmpty(t, spaceID, "space ID should be available before copy")
		require.NotEmpty(t, createdNodeToken, "node token should be available before copy")

		copiedNode := createWikiNode(t, ctx, clie2e.Request{
			Args:      []string{"wiki", "nodes", "copy"},
			DefaultAs: "bot",
			Params: map[string]any{
				"space_id":   spaceID,
				"node_token": createdNodeToken,
			},
			Data: map[string]any{
				"target_space_id": spaceID,
				"title":           copiedTitle,
			},
		})

		copiedNodeToken = copiedNode.Get("node_token").String()
		require.NotEmpty(t, copiedNodeToken)
		assert.Equal(t, copiedTitle, copiedNode.Get("title").String())
		assert.Equal(t, spaceID, copiedNode.Get("space_id").String())
		assert.NotEqual(t, createdNodeToken, copiedNodeToken)
	})

	t.Run("list nodes and find copied node", func(t *testing.T) {
		require.NotEmpty(t, spaceID, "space ID should be available before second list")
		require.NotEmpty(t, copiedNodeToken, "copied node token should be available before second list")

		nodeItem := findWikiNodeByToken(t, ctx, spaceID, copiedNodeToken)
		assert.Equal(t, copiedTitle, nodeItem.Get("title").String())
	})
}
