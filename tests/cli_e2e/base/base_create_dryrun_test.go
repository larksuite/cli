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

func TestBaseCreateDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+base-create",
			"--name", "Project Tracker",
			"--table-name", "Tasks",
			"--time-zone", "Asia/Shanghai",
			"--fields", `[{"name":"Title","type":"text"},{"name":"Status","type":"text"}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "Project Tracker", clie2e.DryRunGet(out, "api.0.body.name").String(), out)
	require.Equal(t, "Asia/Shanghai", clie2e.DryRunGet(out, "api.0.body.time_zone").String(), out)

	require.Equal(t, "/open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables", clie2e.DryRunGet(out, "api.1.url").String(), out)
	require.Equal(t, "GET", clie2e.DryRunGet(out, "api.1.method").String(), out)
	require.Equal(t, int64(0), clie2e.DryRunGet(out, "api.1.params.offset").Int(), out)
	require.Equal(t, int64(100), clie2e.DryRunGet(out, "api.1.params.limit").Int(), out)

	require.Equal(t, "/open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables", clie2e.DryRunGet(out, "api.2.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.2.method").String(), out)
	require.Equal(t, "Tasks", clie2e.DryRunGet(out, "api.2.body.name").String(), out)
	require.Equal(t, "Title", clie2e.DryRunGet(out, "api.2.body.fields.0.name").String(), out)
	require.Equal(t, "Status", clie2e.DryRunGet(out, "api.2.body.fields.1.name").String(), out)

	require.Equal(t, "/open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables/%3Cdefault_table_id%3E", clie2e.DryRunGet(out, "api.3.url").String(), out)
	require.Equal(t, "DELETE", clie2e.DryRunGet(out, "api.3.method").String(), out)
}

func TestBaseCreateDryRunTableNameOnlyRenamesDefaultTable(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+base-create",
			"--name", "Project Tracker",
			"--table-name", "Tasks",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "Project Tracker", clie2e.DryRunGet(out, "api.0.body.name").String(), out)

	require.Equal(t, "/open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables", clie2e.DryRunGet(out, "api.1.url").String(), out)
	require.Equal(t, "GET", clie2e.DryRunGet(out, "api.1.method").String(), out)

	require.Equal(t, "/open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables/%3Cdefault_table_id%3E", clie2e.DryRunGet(out, "api.2.url").String(), out)
	require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.2.method").String(), out)
	require.Equal(t, "Tasks", clie2e.DryRunGet(out, "api.2.body.name").String(), out)
	require.False(t, clie2e.DryRunGet(out, "api.3").Exists(), out)
}

func TestBaseCreateDryRunFieldsOnlyUsesDefaultTableName(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+base-create",
			"--name", "Project Tracker",
			"--fields", `[{"name":"Title","type":"text"}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/%3Ccreated_base_token%3E/tables", clie2e.DryRunGet(out, "api.2.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.2.method").String(), out)
	require.Equal(t, "Table 1", clie2e.DryRunGet(out, "api.2.body.name").String(), out)
	require.Equal(t, "Title", clie2e.DryRunGet(out, "api.2.body.fields.0.name").String(), out)
}
