// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestAppsListDryRun pins cursor-pagination params: default page_size=20 is
// always written; empty --page-token is omitted; negative page_size is passed
// through unchanged (server is the source of truth for range validation).
func TestAppsListDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("DefaultPageSize", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "GET", gjson.Get(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps", gjson.Get(result.Stdout, "api.0.url").String())
		assert.Equal(t, "20", gjson.Get(result.Stdout, "api.0.params.page_size").String())
		assert.False(t, gjson.Get(result.Stdout, "api.0.params.page_token").Exists(),
			"empty page_token must be omitted")
	})

	t.Run("CustomPageSize", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list", "--page-size", "50", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "50", gjson.Get(result.Stdout, "api.0.params.page_size").String())
	})

	t.Run("WithPageToken", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list", "--page-token", "cursor_abc", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "cursor_abc", gjson.Get(result.Stdout, "api.0.params.page_token").String())
		assert.Equal(t, "20", gjson.Get(result.Stdout, "api.0.params.page_size").String())
	})

	t.Run("NegativePageSizePassesThrough", func(t *testing.T) {
		// By design CLI does not bound page_size; server validates. Test pins that
		// invariant so a well-meaning client-side check doesn't sneak in.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list", "--page-size", "-1", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "-1", gjson.Get(result.Stdout, "api.0.params.page_size").String())
	})
}
