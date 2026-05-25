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

func setWikiNodeCreateDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "wiki_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "wiki_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

// TestWikiNodeCreateDryRun pins the request shape and Validate behavior for
// `wiki +node-create`.
func TestWikiNodeCreateDryRun(t *testing.T) {
	setWikiNodeCreateDryRunEnv(t)

	t.Run("HappyPath_ExplicitSpaceID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-create",
				"--space-id", "123456",
				"--title", "TestDoc",
				"--obj-type", "docx",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/wiki/v2/spaces/123456/nodes", gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, "origin", gjson.Get(result.Stdout, "api.0.body.node_type").String())
		assert.Equal(t, "docx", gjson.Get(result.Stdout, "api.0.body.obj_type").String())
		assert.Equal(t, "TestDoc", gjson.Get(result.Stdout, "api.0.body.title").String())
	})

	t.Run("HappyPath_WithParentNodeToken", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-create",
				"--space-id", "123456",
				"--parent-node-token", "wikcnABC123",
				"--title", "ChildDoc",
				"--obj-type", "docx",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// 2-step: resolve parent node -> create node
		assert.Equal(t, "GET", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/wiki/v2/spaces/get_node", gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, "wikcnABC123", gjson.Get(result.Stdout, "api.0.params.token").String())

		assert.Equal(t, "POST", gjson.Get(result.Stdout, "api.1.method").String())
		assert.Equal(t, "/open-apis/wiki/v2/spaces/123456/nodes", gjson.Get(result.Stdout, "api.1.url").String())
		assert.Equal(t, "wikcnABC123", gjson.Get(result.Stdout, "api.1.body.parent_node_token").String())
	})

	t.Run("HappyPath_ShortcutNodeType", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-create",
				"--space-id", "123456",
				"--node-type", "shortcut",
				"--origin-node-token", "wikcnORIG",
				"--title", "ShortcutDoc",
				"--obj-type", "docx",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "shortcut", gjson.Get(result.Stdout, "api.0.body.node_type").String())
		assert.Equal(t, "wikcnORIG", gjson.Get(result.Stdout, "api.0.body.origin_node_token").String())
	})

	t.Run("RejectsShortcutWithoutOriginNodeToken", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-create",
				"--space-id", "123456",
				"--node-type", "shortcut",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateWikiErrorMessage(result)
		assert.Contains(t, msg, "--origin-node-token is required")
	})

	t.Run("RejectsOriginNodeTokenWithoutShortcutType", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-create",
				"--space-id", "123456",
				"--origin-node-token", "wikcnORIG",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateWikiErrorMessage(result)
		assert.Contains(t, msg, "--origin-node-token can only be used when --node-type=shortcut")
	})

	t.Run("RejectsBotWithoutSpaceIDOrParentNodeToken", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-create",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateWikiErrorMessage(result)
		assert.Contains(t, msg, "bot identity requires --space-id or --parent-node-token")
	})

	t.Run("RejectsBotWithMyLibrarySpaceID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-create",
				"--space-id", "my_library",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateWikiErrorMessage(result)
		assert.Contains(t, msg, "bot identity does not support --space-id my_library")
	})

	t.Run("RejectsInvalidObjType", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+node-create",
				"--space-id", "123456",
				"--obj-type", "pdf",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateWikiErrorMessage(result)
		assert.Contains(t, msg, `"pdf"`)
		assert.Contains(t, msg, "invalid value")
	})
}

func validateWikiErrorMessage(r *clie2e.Result) string {
	if msg := gjson.Get(r.Stdout, "error.message").String(); msg != "" {
		return msg
	}
	if msg := gjson.Get(r.Stderr, "error.message").String(); msg != "" {
		return msg
	}
	return r.Stdout + r.Stderr
}
