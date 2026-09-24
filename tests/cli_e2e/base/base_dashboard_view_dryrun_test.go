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

func TestBaseDashboardViewDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	workDir := t.TempDir()
	run := func(t *testing.T, code int, args ...string) *clie2e.Result {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append(args, "--dry-run"), DefaultAs: "bot", WorkDir: workDir})
		require.NoError(t, err)
		result.AssertExitCode(t, code)
		return result
	}
	createArgs := []string{"base", "+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Task view", "--type", "view"}
	for _, config := range []string{
		`{"table_name":"Tasks","view_type":"grid","filter":{"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":"Doing"}]},"group_by":[{"field_name":"Status","mode":"integrated","sort":{"type":"group","order":"asc"}}],"sort_by":[{"field_name":"End","order":"asc"}],"visible_fields":["Title","Status","End"]}`,
		`{"table_name":"Tasks","view_type":"gantt"}`,
		`{"table_name":"Tasks","view_type":"gallery"}`,
	} {
		t.Run(gjson.Get(config, "view_type").String(), func(t *testing.T) {
			path := filepath.Join(workDir, gjson.Get(config, "view_type").String()+".json")
			require.NoError(t, os.WriteFile(path, []byte(config), 0600))
			for _, input := range []string{config, "@" + path} {
				args := append(append([]string{}, createArgs...), "--data-config", input, "--position", `{"x":0,"y":0,"w":12,"h":8}`)
				result := run(t, 0, args...)
				require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
				require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
				require.Equal(t, "view", clie2e.DryRunGet(result.Stdout, "api.0.body.type").String())
				require.JSONEq(t, config, clie2e.DryRunGet(result.Stdout, "api.0.body.data_config").Raw)
				require.Equal(t, int64(12), clie2e.DryRunGet(result.Stdout, "api.0.body.position.w").Int())
			}
		})
	}
	t.Run("partial updates", func(t *testing.T) {
		for _, patch := range []string{
			`{}`, `{"filter":null}`, `{"group_by":[]}`, `{"sort_by":[]}`,
			`{"filter":{"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":"Done"}]}}`,
			`{"group_by":[{"field_name":"Status","mode":"integrated","sort":{"type":"group","order":"desc"}}]}`,
			`{"sort_by":[{"field_name":"End","order":"desc"}]}`, `{"visible_fields":["Title","End"]}`,
			`{"view_type":"gantt"}`, `{"view_type":"gallery"}`, `{"visible_fields":["timebar","card"]}`,
		} {
			result := run(t, 0, "base", "+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_view", "--data-config", patch)
			require.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_view", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.JSONEq(t, patch, clie2e.DryRunGet(result.Stdout, "api.0.body.data_config").Raw)
			require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.type").Exists())
		}
	})
	t.Run("unsupported write fields", func(t *testing.T) {
		for _, tc := range []struct{ name, field, config string }{
			{"timebar object", "timebar", `{"timebar":{"start_time":"Start","end_time":"End","title":"Title"}}`},
			{"timebar empty", "timebar", `{"timebar":{}}`},
			{"timebar null", "timebar", `{"timebar":null}`},
			{"card object", "card", `{"card":{"cover_field":"Cover"}}`},
			{"card empty", "card", `{"card":{}}`},
			{"card null", "card", `{"card":null}`},
		} {
			for _, command := range []string{"+dashboard-block-create", "+dashboard-block-update"} {
				t.Run(tc.name+"/"+command, func(t *testing.T) {
					args := []string{"base", command, "--base-token", "app_x", "--dashboard-id", "dsh_x"}
					config := tc.config
					if command == "+dashboard-block-create" {
						args = append(args, "--name", "Task view", "--type", "view")
						config = `{"table_name":"Tasks","view_type":"grid",` + config[1:]
					} else {
						args = append(args, "--block-id", "blk_view")
					}
					path := filepath.Join(workDir, "unsupported.json")
					require.NoError(t, os.WriteFile(path, []byte(config), 0600))
					for _, input := range []string{config, "@" + path} {
						inputArgs := append(append([]string{}, args...), "--data-config", input)
						result := run(t, 2, inputArgs...)
						require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
						require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
						require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String())
						require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "data_config."+tc.field+" 不支持")
						require.Empty(t, result.Stdout)
						result = run(t, 0, append(inputArgs, "--no-validate")...)
						require.JSONEq(t, config, clie2e.DryRunGet(result.Stdout, "api.0.body.data_config").Raw)
					}
				})
			}
		}
	})
	t.Run("invalid configuration", func(t *testing.T) {
		for _, config := range []string{
			`{}`, `{"view_type":"grid"}`, `{"table_name":"Tasks"}`,
			`{"table_name":"Tasks","view_type":"grid","count_all":true}`,
			`{"table_name":"Tasks","view_type":"grid","sort_by":[{"field_name":"End","order":"invalid"}]}`,
		} {
			result := run(t, 2, append(append([]string{}, createArgs...), "--data-config", config)...)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
			require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String())
			require.Empty(t, result.Stdout)
		}
	})
	t.Run("no validate still parses JSON", func(t *testing.T) {
		args := append(append([]string{}, createArgs...), "--no-validate", "--data-config")
		const future = `{"view_type":false,"future_option":0}`
		result := run(t, 0, append(args, future)...)
		require.JSONEq(t, future, clie2e.DryRunGet(result.Stdout, "api.0.body.data_config").Raw)
		for _, invalid := range []string{`{"view_type":`, `[]`, `null`} {
			result = run(t, 2, append(args, invalid)...)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
		}
	})
	t.Run("get configuration", func(t *testing.T) {
		result := run(t, 0, "base", "+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_view")
		require.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		require.Equal(t, "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_view", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	})
	t.Run("help discovers view guide", func(t *testing.T) {
		for _, command := range []string{"+dashboard-block-create", "+dashboard-block-update", "+dashboard-block-get"} {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"base", command, "--help"}, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Contains(t, result.Stdout, "lark-base-dashboard-view.md")
			if command != "+dashboard-block-get" {
				require.Contains(t, result.Stdout, "reject top-level data_config.timebar and data_config.card")
			} else {
				require.Contains(t, result.Stdout, "tableId, viewId, and viewType")
				require.Contains(t, result.Stdout, "viewType is a string")
				require.Contains(t, result.Stdout, "+view-get-*")
			}
		}
	})
}
