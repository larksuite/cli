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

func TestBaseFieldCreateDryRunArrayCompat(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+field-create",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--json", `[{"name":"A","type":"text"},{"name":"B","type":"text"}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "A", clie2e.DryRunGet(out, "api.0.body.name").String(), out)
	require.Equal(t, "text", clie2e.DryRunGet(out, "api.0.body.type").String(), out)

	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields", clie2e.DryRunGet(out, "api.1.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.1.method").String(), out)
	require.Equal(t, "B", clie2e.DryRunGet(out, "api.1.body.name").String(), out)
	require.Equal(t, "text", clie2e.DryRunGet(out, "api.1.body.type").String(), out)
}

func TestBaseFieldButtonWorkflowDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cases := []struct {
		name   string
		args   []string
		method string
		url    string
	}{
		{
			name: "bind",
			args: []string{
				"base", "+field-button-bind",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_x",
				"--workflow-id", "wkf_x",
				"--dry-run",
			},
			method: "POST",
			url:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/button-workflow:bind",
		},
		{
			name: "get field binding",
			args: []string{
				"base", "+field-button-binding-get",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_x",
				"--dry-run",
			},
			method: "GET",
			url:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/button-workflow",
		},
		{
			name: "unbind",
			args: []string{
				"base", "+field-button-unbind",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_x",
				"--dry-run",
			},
			method: "POST",
			url:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/button-workflow:unbind",
		},
		{
			name: "workflow button fields",
			args: []string{
				"base", "+workflow-button-fields",
				"--base-token", "app_x",
				"--workflow-id", "wkf_x",
				"--dry-run",
			},
			method: "GET",
			url:    "/open-apis/base/v3/bases/app_x/workflows/wkf_x/button-fields",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tt.args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, tt.url, clie2e.DryRunGet(out, "api.0.url").String(), out)
			require.Equal(t, tt.method, clie2e.DryRunGet(out, "api.0.method").String(), out)
			if tt.name == "bind" {
				require.Equal(t, "wkf_x", clie2e.DryRunGet(out, "api.0.body.workflow_id").String(), out)
			}
		})
	}
}
