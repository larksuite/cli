// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseDashboardButtonDryRun(t *testing.T) {
	cases := []struct {
		name    string
		command string
		args    []string
		method  string
		path    string
		body    string
	}{
		{
			name: "create", command: "+dashboard-block-create",
			args:   []string{"--name", "Run workflow", "--type", "button"},
			method: "POST", path: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks",
			body: `{"name":"Run workflow","type":"button","data_config":{"label":"  Run workflow  "}}`,
		},
		{
			name: "update label", command: "+dashboard-block-update",
			args:   []string{"--block-id", "blk_x"},
			method: "PATCH", path: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x",
			body: `{"data_config":{"label":"  Run workflow  "}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setBaseDryRunConfigEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			args := []string{"base", tc.command, "--base-token", "app_x", "--dashboard-id", "dsh_x",
				"--data-config", `{"label":"  Run workflow  "}`, "--dry-run"}
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append(args, tc.args...), DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, tc.method, clie2e.DryRunGet(result.Stdout, "api.0.method").String(), result.Stdout)
			require.Equal(t, tc.path, clie2e.DryRunGet(result.Stdout, "api.0.url").String(), result.Stdout)
			require.JSONEq(t, tc.body, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw)
		})
	}
}
