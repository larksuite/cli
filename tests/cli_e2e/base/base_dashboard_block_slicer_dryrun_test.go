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

func TestBaseDashboardBlockSlicerDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	const minimal = `{"table_name":"Orders","group_by":[{"field_name":"Region"}]}`
	const defaults = `{"table_name":"Orders","group_by":[{"field_name":"Region"}],"displayMode":"tiled","selectMode":"multiple","order":"asc"}`
	for _, tc := range []struct {
		name       string
		create     bool
		input      string
		want       string
		noValidate bool
	}{
		{"create defaults", true, minimal, defaults, false},
		{"create bypass", true, minimal, minimal, true},
		{"sort patch", false, `{"order":"desc"}`, `{"order":"desc"}`, false},
		{"clear filter", false, `{"filter":null}`, `{"filter":null}`, false},
		{"change table", false, `{"table_name":"Customers","group_by":[{"field_name":"City"}]}`, `{"table_name":"Customers","group_by":[{"field_name":"City"}]}`, false},
		{"empty patch", false, `{}`, `{}`, false},
	} {
		for _, fromFile := range []bool{false, true} {
			label := "inline"
			if fromFile {
				label = "file"
			}
			t.Run(tc.name+"/"+label, func(t *testing.T) {
				input := tc.input
				workDir := ""
				if fromFile {
					workDir = t.TempDir()
					path := filepath.Join(workDir, "config.json")
					require.NoError(t, os.WriteFile(path, []byte(input), 0o600))
					input = "@config.json"
				}
				args := []string{"base", "+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--data-config", input, "--dry-run"}
				method, url := "PATCH", "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks"
				if tc.create {
					args[1] = "+dashboard-block-create"
					args = append(args, "--type", "slicer", "--name", "Region Filter")
					method = "POST"
				} else {
					args = append(args, "--block-id", "blk_x")
					url += "/blk_x"
				}
				if tc.noValidate {
					args = append(args, "--no-validate")
				}
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot", WorkDir: workDir})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)
				require.Len(t, clie2e.DryRunGet(result.Stdout, "api").Array(), 1)
				require.Equal(t, method, clie2e.DryRunGet(result.Stdout, "api.0.method").String())
				require.Equal(t, url, clie2e.DryRunGet(result.Stdout, "api.0.url").String())
				require.JSONEq(t, tc.want, clie2e.DryRunGet(result.Stdout, "api.0.body.data_config").Raw)
				if tc.create {
					require.Equal(t, "slicer", clie2e.DryRunGet(result.Stdout, "api.0.body.type").String())
				} else {
					require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.type").Exists())
				}
			})
		}
	}
}

func TestBaseDashboardBlockSlicerDryRunRejectsInvalidConfig(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	for _, input := range []string{
		`{"table_name":"Orders","group_by":[{"field_name":"Region"}],"count_all":true}`,
		`{"table_name":"Orders","group_by":[{"field_name":"Region"}],"displayMode":"dropdown"}`,
		`{"table_name":"Orders","group_by":[{"field_name":"Region"}],"displayMode":"tiled","selectMode":"multiple","order":1}`,
		`{"table_name":"Orders","group_by":[{"field_name":"Region"}],"filter":null}`,
		`{"table_name":"Orders","group_by":[]}`,
		`null`,
		`[]`,
	} {
		t.Run(input, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"base", "+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Invalid", "--type", "slicer", "--data-config", input, "--dry-run"}, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
			require.Equal(t, "--data-config", gjson.Get(result.Stderr, "error.param").String())
			require.Empty(t, result.Stdout)
		})
	}
}
