// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseDashboardLinkageDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	for _, input := range []string{`{"target_block_ids":["chtcnB","chtcnC"]}`, `{"target_block_ids":[]}`} {
		for _, fromFile := range []bool{false, true} {
			t.Run(input+map[bool]string{false: "/inline", true: "/file"}[fromFile], func(t *testing.T) {
				value, workDir := input, ""
				if fromFile {
					workDir = t.TempDir()
					require.NoError(t, os.WriteFile(filepath.Join(workDir, "linkage.json"), []byte(input), 0o600))
					value = "@linkage.json"
				}
				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:      []string{"base", "+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "chtcnA", "--linkage", value, "--dry-run"},
					DefaultAs: "bot", WorkDir: workDir,
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)
				require.Len(t, clie2e.DryRunGet(result.Stdout, "api").Array(), 1)
				require.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
				require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/chtcnA", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
				require.JSONEq(t, `{"linkage":`+input+`}`, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw)
			})
		}
	}
	t.Run("omission preserves linkage", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"base", "+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "chtcnA", "--name", "Regions", "--dry-run"}, DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.JSONEq(t, `{"name":"Regions"}`, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw)
	})
	t.Run("help exposes linkage and guide", func(t *testing.T) {
		for _, args := range [][]string{{"base", "+dashboard-block-update", "--help"}, {"base", "+dashboard-block-get", "--help"}} {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Contains(t, result.Stdout, "linkage")
			require.Contains(t, result.Stdout, "lark-base-dashboard-linkage.md")
		}
	})
}

func TestBaseDashboardLinkageDryRunRejectsInvalidInput(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	for _, input := range []string{"", "null", "{}", `{"target_block_ids":null}`, `{"target_block_ids":["chtcnA"]}`, `{"target_block_ids":["chtcnB","chtcnB"]}`, `{"target_block_ids":[],"field_mappings":[]}`, `{"target_block_ids":[]} {}`} {
		t.Run(input, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"base", "+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "chtcnA", "--linkage", input, "--no-validate", "--dry-run"}, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
			require.Equal(t, "--linkage", gjson.Get(result.Stderr, "error.param").String())
			require.Empty(t, result.Stdout)
		})
	}
}
