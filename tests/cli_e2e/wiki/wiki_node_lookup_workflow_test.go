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

func TestWiki_MutationNodeLookupWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	spaceID := getWikiSpace(t, ctx, "my_library").Get("space_id").String()
	require.NotEmpty(t, spaceID)
	parent, result, err := createWikiNode(t, t, ctx, spaceID, map[string]any{
		"node_type": "origin", "obj_type": "docx", "title": "lark-cli-e2e-node-lookup-" + clie2e.GenerateSuffix(),
	})
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode, "%s", result.Stderr)
	parentToken := parent.Get("node_token").String()
	target, result, err := createWikiNode(t, t, ctx, spaceID, map[string]any{
		"node_type": "origin", "obj_type": "docx", "title": "lookup target", "parent_node_token": parentToken,
	})
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode, "%s", result.Stderr)
	targetToken := target.Get("node_token").String()

	for _, objType := range []string{"wiki", "docx"} {
		t.Run("delete by "+objType, func(t *testing.T) {
			created, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"wiki", "+node-create", "--parent-node-token", parent.Get("obj_token").String(), "--title", "lookup child"}, DefaultAs: "bot",
			})
			require.NoError(t, err)
			require.Equal(t, 0, created.ExitCode, "%s", created.Stderr)
			node := gjson.Get(created.Stdout, "data")
			nodeToken, objToken := node.Get("node_token").String(), node.Get("obj_token").String()
			require.NotEmpty(t, nodeToken)
			require.NotEmpty(t, objToken)
			t.Cleanup(func() {
				cleanupCtx, cancel := clie2e.CleanupContext()
				defer cancel()
				result, err := deleteWikiNodeAndVerify(cleanupCtx, spaceID, nodeToken, "docx")
				clie2e.ReportCleanupFailure(t, "delete lookup child", result, err)
			})
			require.Equal(t, spaceID, node.Get("space_id").String())
			require.Equal(t, parentToken, node.Get("parent_node_token").String())

			// Compare both lookup APIs on the isolated fixture, including obj_token
			// resolution used by deletion. The old endpoint remains a supported API.
			for _, token := range []string{nodeToken, objToken} {
				var previous gjson.Result
				for _, endpoint := range []string{"get_node", "node_by_token"} {
					params := map[string]any{"token": token}
					if endpoint == "get_node" && token == objToken {
						params["obj_type"] = "docx"
					}
					lookup, err := clie2e.RunCmd(ctx, clie2e.Request{
						Args: []string{"api", "get", "/open-apis/wiki/v2/spaces/" + endpoint}, Params: params, DefaultAs: "bot",
					})
					require.NoError(t, err)
					require.Equal(t, 0, lookup.ExitCode, "%s", lookup.Stderr)
					current := gjson.Get(lookup.Stdout, "data.node")
					require.Equal(t, nodeToken, current.Get("node_token").String())
					if previous.Exists() {
						for _, field := range []string{"node_token", "obj_token", "obj_type", "space_id", "parent_node_token", "node_type", "origin_node_token"} {
							require.Equal(t, previous.Get(field).String(), current.Get(field).String(), field)
						}
					}
					previous = current
				}
			}

			// Resolve both source and target, moving only within the isolated tree.
			moved, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"wiki", "+move", "--node-token", objToken, "--target-parent-token", target.Get("obj_token").String()}, DefaultAs: "bot",
			})
			require.NoError(t, err)
			require.Equal(t, 0, moved.ExitCode, "%s", moved.Stderr)
			require.Equal(t, targetToken, gjson.Get(moved.Stdout, "data.parent_node_token").String())
			require.Equal(t, targetToken, getWikiNode(t, ctx, nodeToken).Get("parent_node_token").String())
			back, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"wiki", "+move", "--node-token", objToken, "--source-space-id", spaceID, "--target-parent-token", parent.Get("obj_token").String()}, DefaultAs: "bot",
			})
			require.NoError(t, err)
			require.Equal(t, 0, back.ExitCode, "%s", back.Stderr)
			require.Equal(t, parentToken, gjson.Get(back.Stdout, "data.parent_node_token").String())
			require.Equal(t, parentToken, getWikiNode(t, ctx, nodeToken).Get("parent_node_token").String())

			// A shortcut's object token identifies this original document. Reject
			// converting the shortcut into a document deletion, then delete only
			// the shortcut and independently verify that the original survives.
			if objType == "wiki" {
				shortcut, createdShortcut, err := createWikiNode(t, t, ctx, spaceID, map[string]any{
					"node_type": "shortcut", "obj_type": "docx", "origin_node_token": nodeToken, "parent_node_token": parentToken,
				})
				require.NoError(t, err)
				require.Equal(t, 0, createdShortcut.ExitCode, "%s", createdShortcut.Stderr)
				shortcutToken := shortcut.Get("node_token").String()
				blocked, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{"wiki", "+node-delete", "--node-token", shortcutToken, "--obj-type", "docx", "--yes"}, DefaultAs: "bot",
				})
				require.NoError(t, err)
				require.Equal(t, 2, blocked.ExitCode, "%s", blocked.Stderr)
				require.Equal(t, "--obj-type", gjson.Get(blocked.Stderr, "error.param").String())
				require.Equal(t, shortcutToken, getWikiNode(t, ctx, shortcutToken).Get("node_token").String())
				require.Equal(t, nodeToken, getWikiNode(t, ctx, nodeToken).Get("node_token").String())
				deletedShortcut, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{"wiki", "+node-delete", "--node-token", shortcutToken, "--obj-type", "wiki", "--yes"}, DefaultAs: "bot",
				})
				require.NoError(t, err)
				require.Equal(t, 0, deletedShortcut.ExitCode, "%s", deletedShortcut.Stderr)
				require.NoError(t, waitWikiNodeDeleted(ctx, shortcutToken))
				require.Equal(t, nodeToken, getWikiNode(t, ctx, nodeToken).Get("node_token").String())
			}
			token := objToken
			if objType == "docx" {
				token = nodeToken
			}
			deleted, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{"wiki", "+node-delete", "--node-token", token, "--obj-type", objType, "--yes"}, DefaultAs: "bot",
			})
			require.NoError(t, err)
			require.Equal(t, 0, deleted.ExitCode, "%s", deleted.Stderr)
			require.Equal(t, spaceID, gjson.Get(deleted.Stdout, "data.space_id").String())
			require.Equal(t, objType, gjson.Get(deleted.Stdout, "data.obj_type").String())
			// Task completion can precede lookup visibility. Poll the new
			// endpoint itself; unrelated errors must still fail immediately.
			visibilityCtx, cancel := context.WithTimeout(ctx, wikiDeleteVisibilityTimeout)
			defer cancel()
			require.NoError(t, clie2e.WaitForCondition(visibilityCtx, clie2e.WaitOptions{
				Timeout: wikiDeleteVisibilityTimeout, Interval: wikiDeleteVisibilityPoll,
			}, func() (bool, error) {
				missing, err := clie2e.RunCmd(visibilityCtx, clie2e.Request{
					Args:   []string{"api", "get", "/open-apis/wiki/v2/spaces/node_by_token"},
					Params: map[string]any{"token": nodeToken}, DefaultAs: "bot",
				})
				if err != nil {
					return false, err
				}
				if missing.ExitCode == 0 {
					require.Equal(t, nodeToken, gjson.Get(missing.Stdout, "data.node.node_token").String())
					return false, nil
				}
				require.Equal(t, 1, missing.ExitCode, "%s", missing.Stderr)
				require.Contains(t, []int64{131005, 131012}, gjson.Get(missing.Stderr, "error.code").Int())
				return true, nil
			}))
		})
	}
}
