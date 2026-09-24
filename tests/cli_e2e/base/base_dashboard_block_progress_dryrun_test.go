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

func runProgressDryRun(t *testing.T, wantExit int, args ...string) *clie2e.Result {
	t.Helper()
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append(args, "--dry-run"), DefaultAs: "bot"})
	require.NoError(t, err)
	result.AssertExitCode(t, wantExit)
	return result
}

func TestBaseDashboardProgressCreateDryRun(t *testing.T) {
	// Go 1.23 includes subtest names in t.TempDir paths, so keep them short.
	for _, tc := range []struct {
		name   string
		config string
	}{
		{
			name:   "custom_value",
			config: `{"progress":{"type":"value","current":{"type":"custom","value":0},"target":{"type":"custom","value":-0.5}}}`,
		},
		{
			name:   "field_value_with_filter",
			config: `{"progress":{"type":"value","current":{"type":"field","table_name":"Orders","count_all":true},"target":{"type":"field","table_name":"Goals","series":[{"field_name":"Amount","rollup":"SUM"}],"filter":{"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":["Done"]}]}}}}`,
		},
		{
			name:   "relative_date",
			config: `{"progress":{"type":"date","current":{"type":"relative","value":"today"},"target":{"type":"relative","value":"month"}}}`,
		},
		{
			name:   "custom_date_range",
			config: `{"progress":{"type":"date","current":{"type":"custom","value":0},"target":{"type":"custom","range":[-1,1]},"dateFormat":"MM/DD/YYYY"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runProgressDryRun(t, 0, "base", "+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Progress", "--type", "progress", "--data-config", tc.config)
			require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Equal(t, "progress", clie2e.DryRunGet(result.Stdout, "api.0.body.type").String())
			require.JSONEq(t, tc.config, clie2e.DryRunGet(result.Stdout, "api.0.body.data_config").Raw)
		})
	}
}

func TestBaseDashboardProgressUpdateDryRun(t *testing.T) {
	for _, patch := range []string{
		`{}`, `{"progress":{}}`, `{"progress":{"target":{"value":0}}}`,
		`{"progress":{"current":{"filter":null}}}`,
		`{"progress":{"current":{"filter":{"conjunction":"or","conditions":[{"field_name":"Status","operator":"is","value":["Done"]}]}}}}`,
		`{"progress":{"current":{"table_name":"Goals","count_all":true}}}`,
		`{"progress":{"target":{"type":"custom","value":50}}}`,
		`{"progress":{"dateFormat":"YYYY/MM/DD"}}`,
		`{"progress":{"type":"date","current":{"type":"relative","value":"yesterday"},"target":{"type":"relative","value":"quarter"}}}`,
	} {
		t.Run(patch, func(t *testing.T) {
			result := runProgressDryRun(t, 0, "base", "+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x", "--data-config", patch)
			require.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.JSONEq(t, patch, clie2e.DryRunGet(result.Stdout, "api.0.body.data_config").Raw)
			require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.type").Exists())
		})
	}
}

func TestBaseDashboardProgressInvalidCreateDryRun(t *testing.T) {
	for _, config := range []string{
		`{}`, `{"progress":{"type":"value","target":{"type":"custom","value":1}}}`,
		`{"progress":{"type":"value","current":{"type":"field","table_name":"Orders","count_all":true,"filter":null},"target":{"type":"custom","value":1}}}`,
		`{"progress":{"type":"date","current":{"type":"relative","value":"today"},"target":{"type":"custom","range":[2,1]}}}`,
	} {
		t.Run(config, func(t *testing.T) {
			result := runProgressDryRun(t, 2, "base", "+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Invalid progress", "--type", "progress", "--data-config", config)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
			require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String())
			require.Empty(t, result.Stdout)
		})
	}
}

func TestBaseDashboardProgressNoValidateDryRun(t *testing.T) {
	config := `{"progress":{"type":"future","target":{"value":false}}}`
	result := runProgressDryRun(t, 0, "base", "+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Progress", "--type", "progress", "--data-config", config, "--no-validate")
	require.JSONEq(t, config, clie2e.DryRunGet(result.Stdout, "api.0.body.data_config").Raw)
	result = runProgressDryRun(t, 2, "base", "+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Progress", "--type", "progress", "--data-config", `{"progress":`, "--no-validate")
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
	require.Empty(t, result.Stdout)
}

func TestBaseDashboardProgressGetDryRun(t *testing.T) {
	result := runProgressDryRun(t, 0, "base", "+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x")
	require.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
}
