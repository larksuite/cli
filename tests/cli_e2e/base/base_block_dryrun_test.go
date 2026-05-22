// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseBlockDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("list all", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+base-block-list",
				"--base-token", "app_x",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/blocks/list", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), out)
		require.False(t, gjson.Get(out, "api.0.body.parent_id").Exists(), out)
	})

	t.Run("list folder", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+base-block-list",
				"--base-token", "app_x",
				"--parent-id", "blk_folder",
				"--type", "docx",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/blocks/list", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "blk_folder", gjson.Get(out, "api.0.body.parent_id").String(), out)
		require.False(t, gjson.Get(out, "api.0.body.type").Exists(), out)
	})

	t.Run("create", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+base-block-create",
				"--base-token", "app_x",
				"--type", "docx",
				"--name", "Spec",
				"--parent-id", "blk_folder",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/blocks", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), out)
		require.Equal(t, "docx", gjson.Get(out, "api.0.body.type").String(), out)
		require.Equal(t, "Spec", gjson.Get(out, "api.0.body.name").String(), out)
		require.Equal(t, "blk_folder", gjson.Get(out, "api.0.body.parent_id").String(), out)
	})

	t.Run("move root", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+base-block-move",
				"--base-token", "app_x",
				"--block-id", "blk_a",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/blocks/blk_a/move", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), out)
		require.True(t, gjson.Get(out, "api.0.body.parent_id").Exists(), out)
		require.Equal(t, "Null", gjson.Get(out, "api.0.body.parent_id").Type.String(), out)
	})

	t.Run("move after", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+base-block-move",
				"--base-token", "app_x",
				"--block-id", "blk_a",
				"--parent-id", "blk_folder",
				"--after-id", "blk_b",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/blocks/blk_a/move", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "blk_folder", gjson.Get(out, "api.0.body.parent_id").String(), out)
		require.Equal(t, "blk_b", gjson.Get(out, "api.0.body.after_id").String(), out)
	})

	t.Run("rename", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+base-block-rename",
				"--base-token", "app_x",
				"--block-id", "blk_a",
				"--name", "Renamed",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/blocks/blk_a/rename", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), out)
		require.Equal(t, "Renamed", gjson.Get(out, "api.0.body.name").String(), out)
	})

	t.Run("delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+base-block-delete",
				"--base-token", "app_x",
				"--block-id", "blk_a",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/blocks/blk_a", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "DELETE", gjson.Get(out, "api.0.method").String(), out)
	})
}
