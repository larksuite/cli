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

func TestDashboardCountdownCreateDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	for _, config := range []string{
		`{"type":"fixed","targetTime":0}`,
		`{"type":"field","aggregateType":"MIN","additionalDatasources":[{"tableName":"Tasks","countAll":true,"groupBy":[{"fieldName":"Deadline","sort":{"type":"group"}}]}]}`,
	} {
		for _, source := range []string{"inline", "file"} {
			t.Run(config+source, func(t *testing.T) {
				workDir := t.TempDir()
				input := config
				if source == "file" {
					path := filepath.Join(workDir, "countdown.json")
					require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
					input = "@countdown.json"
				}
				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:    []string{"base", "+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Deadline", "--type", "countdown", "--data-config", input, "--dry-run"},
					WorkDir: workDir, DefaultAs: "bot",
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				require.Empty(t, result.Stderr)
				require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
				require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
				require.JSONEq(t, `{"name":"Deadline","type":"countdown","data_config":`+config+`,"fine_grid":true}`, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw)
			})
		}
	}
}

func TestDashboardCountdownUpdateAndGetDryRun(t *testing.T) {
	for _, patch := range []string{
		`{}`, `{"targetTime":0}`, `{"aggregateType":"MAX"}`,
		`{"additionalDatasources":[{"filter":null}]}`,
		`{"additionalDatasources":[{"groupBy":[{"fieldName":"Created"}]}]}`,
		`{"additionalDatasources":[{"filter":{"conjunction":"and","conditions":[{"fieldName":"Done","operator":"is","value":false}]}}]}`,
		`{"type":"field","aggregateType":"MIN","additionalDatasources":[{"tableName":"Tasks","countAll":true,"groupBy":[{"fieldName":"Deadline"}]}]}`,
	} {
		t.Run(patch, func(t *testing.T) {
			result := runBaseDryRun(t, 0, "base", "+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x", "--data-config", patch)
			require.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.JSONEq(t, `{"data_config":`+patch+`}`, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw)
			require.Len(t, clie2e.DryRunGet(result.Stdout, "api").Array(), 1)
		})
	}
	result := runBaseDryRun(t, 0, "base", "+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x", "--user-id-type", "open_id")
	require.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "open_id", clie2e.DryRunGet(result.Stdout, "api.0.params.user_id_type").String())
	require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body").Exists())
}

func TestDashboardCountdownDryRunValidationAndBypass(t *testing.T) {
	args := []string{"base", "+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Deadline", "--type", "countdown", "--data-config", `{"type":"fixed","targetTime":-1}`}
	result := runBaseDryRun(t, 2, args...)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
	require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String())
	require.Empty(t, result.Stdout)
	result = runBaseDryRun(t, 0, append(args, "--no-validate")...)
	require.JSONEq(t, `{"type":"fixed","targetTime":-1}`, clie2e.DryRunGet(result.Stdout, "api.0.body.data_config").Raw)
}

func TestDashboardCountdownUpdateFilePreservesNull(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	workDir := t.TempDir()
	const patch = `{"additionalDatasources":[{"filter":null}]}`
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "patch.json"), []byte(patch), 0o600))
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:    []string{"base", "+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x", "--data-config", "@patch.json", "--dry-run"},
		WorkDir: workDir, DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.JSONEq(t, `{"data_config":`+patch+`}`, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw)
}
