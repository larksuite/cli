// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestWiki_ShortcutWorkflow exercises the shortcut layer (wiki +space-list,
// +node-list, +node-copy) end-to-end against a real Lark tenant. The existing
// TestWiki_NodeWorkflow only hits the bare `api` command, so it does not
// protect against regressions in shortcut-specific behavior — flag → body
// mapping, envelope shape ({spaces|nodes, has_more, page_token} + meta.count),
// auto-pagination, my_library alias resolution, or required-flag validation.
func TestWiki_ShortcutWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	parentTitle := "lark-cli-e2e-wiki-sc-parent-" + suffix
	childTitle := "lark-cli-e2e-wiki-sc-child-" + suffix
	copyTitle := "lark-cli-e2e-wiki-sc-copy-" + suffix

	var spaceID, parentNodeToken, childNodeToken, childObjType string

	// Setup: reuse an existing first-layer node in my_library as the host so
	// we never bump the top-layer node count (the bot's my_library top layer
	// has hit the API's "single-layer nodes ... upper limit" — code 131003 —
	// in earlier CI runs because of leftover nodes). Then create a FRESH
	// intermediate parent under that host, and put the test child under the
	// fresh parent. We can't put the child directly under the host because
	// leftover nodes from prior runs accumulate as the host's children, so
	// `+node-list --parent-node-token=<host>` returns hundreds of unrelated
	// nodes and the just-created child gets paged out (regardless of
	// --page-limit) before the test can find it. An isolated intermediate
	// parent always has exactly the children this test creates, so the
	// pagination scan never has to dig through historical cruft.
	t.Run("setup: locate my_library host node + create isolated parent + create test child", func(t *testing.T) {
		listResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/my_library/nodes"},
			DefaultAs: "bot",
			Params:    map[string]any{"page_size": 50},
		})
		require.NoError(t, err)
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, 0)

		items := gjson.Get(listResult.Stdout, "data.items").Array()
		if len(items) == 0 {
			t.Skip("skipped: my_library has no existing top-level nodes to host the test structure")
		}
		host := items[0]
		spaceID = host.Get("space_id").String()
		hostNodeToken := host.Get("node_token").String()
		require.NotEmpty(t, spaceID, "host space_id must be present in listing")
		require.NotEmpty(t, hostNodeToken, "host node_token must be present in listing")

		// Create a fresh intermediate parent under the host. The helper
		// auto-registers a t.Cleanup callback that deletes this parent
		// (and, by API cascade, anything still under it) after the test.
		isolatedParent := createWikiNode(t, parentT, ctx, spaceID, map[string]any{
			"node_type":         "origin",
			"obj_type":          "docx",
			"title":             parentTitle,
			"parent_node_token": hostNodeToken,
		})
		parentNodeToken = isolatedParent.Get("node_token").String()
		require.NotEmpty(t, parentNodeToken, "isolated parent node_token must be present after create")

		// Create the test child UNDER the freshly-isolated parent.
		child := createWikiNode(t, parentT, ctx, spaceID, map[string]any{
			"node_type":         "origin",
			"obj_type":          "docx",
			"title":             childTitle,
			"parent_node_token": parentNodeToken,
		})
		childNodeToken = child.Get("node_token").String()
		childObjType = child.Get("obj_type").String()
		require.NotEmpty(t, childNodeToken)
	})

	// QA-P1: +space-list envelope shape is stable for JSON consumers.
	// `spaces` must always be an array (never null), and pagination metadata
	// fields must always exist so downstream agents can introspect.
	t.Run("+space-list: stable envelope shape", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"wiki", "+space-list", "--page-size", "1"},
			DefaultAs: "bot",
			Format:    "json",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := gjson.Parse(result.Stdout)
		require.True(t, out.Get("ok").Bool(), "stdout:\n%s", result.Stdout)
		assert.True(t, out.Get("data.spaces").Exists(), "data.spaces must exist")
		assert.True(t, out.Get("data.spaces").IsArray(), "data.spaces must be an array, even when empty")
		assert.True(t, out.Get("data.has_more").Exists(), "data.has_more must always be present")
		assert.True(t, out.Get("data.page_token").Exists(), "data.page_token must always be present")
		// meta.count uses `json:",omitempty"` in the envelope framework, so the
		// field is dropped when the count is zero. Comparing values (gjson
		// returns 0 for missing keys) keeps the assertion correct in both the
		// "no spaces visible" and "some spaces" cases without requiring a
		// framework-level change.
		spacesLen := len(out.Get("data.spaces").Array())
		assert.Equal(t, float64(spacesLen), out.Get("meta.count").Float(),
			"meta.count must equal len(data.spaces) (or be omitted when zero); stdout:\n%s", result.Stdout)
	})

	// QA-P1: +node-list correctly maps flags onto the underlying request body
	// and surfaces the child we just created under the parent.
	t.Run("+node-list: finds child under parent", func(t *testing.T) {
		require.NotEmpty(t, spaceID)
		require.NotEmpty(t, parentNodeToken)
		require.NotEmpty(t, childNodeToken)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-list",
				"--space-id", spaceID,
				"--parent-node-token", parentNodeToken,
				"--page-all",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := gjson.Parse(result.Stdout)
		require.True(t, out.Get("ok").Bool(), "stdout:\n%s", result.Stdout)

		match := out.Get(`data.nodes.#(node_token=="` + childNodeToken + `")`)
		require.True(t, match.Exists(), "+node-list did not return the child we created:\n%s", result.Stdout)
		assert.Equal(t, childTitle, match.Get("title").String())
		assert.Equal(t, parentNodeToken, match.Get("parent_node_token").String())
	})

	// QA-P2: --page-size 1 --page-all --page-limit 1 must aggregate exactly
	// one page and surface the next cursor when has_more=true. This catches
	// regressions where the pagination loop overruns the cap or fails to
	// surface has_more / page_token.
	t.Run("+node-list: --page-limit caps the loop and exposes cursor", func(t *testing.T) {
		require.NotEmpty(t, spaceID)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-list",
				"--space-id", spaceID,
				"--page-size", "1",
				"--page-all",
				"--page-limit", "1",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := gjson.Parse(result.Stdout)
		require.True(t, out.Get("ok").Bool(), "stdout:\n%s", result.Stdout)
		nodes := out.Get("data.nodes").Array()
		assert.LessOrEqual(t, len(nodes), 1, "--page-limit=1 + --page-size=1 should yield ≤1 node, got %d", len(nodes))
		// has_more / page_token must still exist — never elided — so
		// callers can resume regardless of whether the cap actually fired.
		assert.True(t, out.Get("data.has_more").Exists())
		assert.True(t, out.Get("data.page_token").Exists())
	})

	// QA-P1: +node-copy creates a copy under the same space and the source
	// stays put (copy ≠ move). Cleanup deletes the copy. The copy is placed
	// under the same host parent we use for the test child, so it doesn't
	// add another top-layer node and trip the per-space limit.
	t.Run("+node-copy: copies child + verifies source survives + cleanup", func(t *testing.T) {
		require.NotEmpty(t, spaceID)
		require.NotEmpty(t, parentNodeToken)
		require.NotEmpty(t, childNodeToken)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-copy",
				"--space-id", spaceID,
				"--node-token", childNodeToken,
				"--target-parent-node-token", parentNodeToken,
				"--title", copyTitle,
			},
			// +node-copy is now declared high-risk-write to align with the
			// upstream API's `danger: true` flag, so the framework requires
			// explicit confirmation before issuing the request.
			Yes:       true,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := gjson.Parse(result.Stdout)
		require.True(t, out.Get("ok").Bool(), "stdout:\n%s", result.Stdout)
		copiedNodeToken := out.Get("data.node_token").String()
		copiedSpaceID := out.Get("data.space_id").String()
		copiedObjType := out.Get("data.obj_type").String()
		require.NotEmpty(t, copiedNodeToken, "stdout:\n%s", result.Stdout)
		require.NotEmpty(t, copiedSpaceID)
		assert.Equal(t, copyTitle, out.Get("data.title").String())

		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()
			deleteResult, deleteErr := deleteWikiNode(cleanupCtx, copiedSpaceID, copiedNodeToken, copiedObjType)
			clie2e.ReportCleanupFailure(parentT, "delete copied wiki node "+copiedNodeToken, deleteResult, deleteErr)
		})

		// Copy must be retrievable; source must still exist (copy ≠ move).
		copied := getWikiNode(t, ctx, copiedNodeToken)
		assert.Equal(t, copyTitle, copied.Get("title").String())
		original := getWikiNode(t, ctx, childNodeToken)
		assert.Equal(t, childTitle, original.Get("title").String(),
			"source node must remain after +node-copy (copy is non-destructive)")
		_ = childObjType // reserved for future +node-list filter checks
	})

	// QA-P2: bot identity must be rejected upfront when --space-id=my_library
	// because the personal-library alias is per-user and meaningless for a
	// tenant_access_token. The shortcut layer should fail before sending any
	// HTTP request, with a validation error mentioning my_library.
	t.Run("+node-list --space-id my_library --as bot: validation rejection", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"wiki", "+node-list", "--space-id", "my_library"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		assert.NotEqual(t, 0, result.ExitCode, "bot + my_library must fail")

		combined := strings.ToLower(result.Stdout + "\n" + result.Stderr)
		assert.Contains(t, combined, "my_library",
			"error must mention my_library to disambiguate from generic auth failures; got stdout=%s stderr=%s",
			result.Stdout, result.Stderr)
	})

	// QA-P2: user identity must positively resolve --space-id=my_library to a
	// real per-user space_id and proceed to list nodes. Skipped when no user
	// token is available (matches the rest of the suite's user-flow gating).
	t.Run("+node-list --space-id my_library --as user: resolves and lists", func(t *testing.T) {
		clie2e.SkipWithoutUserToken(t)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"wiki", "+node-list", "--space-id", "my_library", "--page-size", "1"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := gjson.Parse(result.Stdout)
		require.True(t, out.Get("ok").Bool(), "stdout:\n%s", result.Stdout)
		assert.True(t, out.Get("data.nodes").Exists(), "data.nodes must exist after my_library resolution")
		assert.True(t, out.Get("data.nodes").IsArray(), "data.nodes must be an array")
		// stderr must record the my_library resolution so users/agents can
		// see what space_id the alias mapped to.
		assert.Contains(t, result.Stderr, "Resolved my_library",
			"expected my_library resolution log on stderr; got: %s", result.Stderr)
	})
}
