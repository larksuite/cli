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

func TestBaseFieldGroupCreateDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+field-group-create",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--json", `{"field_groups":[{"name":"Customer Info","children":[{"type":"field","id":"fldA"}]}]}`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/bitable/v1/apps/app_x/tables/tbl_x/field_groups", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "Customer Info", clie2e.DryRunGet(out, "api.0.body.field_groups.0.name").String(), out)
	require.Equal(t, "field", clie2e.DryRunGet(out, "api.0.body.field_groups.0.children.0.type").String(), out)
	require.Equal(t, "fldA", clie2e.DryRunGet(out, "api.0.body.field_groups.0.children.0.id").String(), out)
}
