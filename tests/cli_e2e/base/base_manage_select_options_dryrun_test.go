// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseManageSelectOptionsDryRun(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+manage-select-options",
		"--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "Status",
		"--json", `{"operations":[{"operation":"add","name":"Done"},{"operation":"rename","name":"Todo","new_name":"Doing"},{"operation":"delete","name":"Old"}]}`,
		"--yes",
	)
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/Status/options", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, "add", clie2e.DryRunGet(result.Stdout, "api.0.body.operations.0.operation").String())
	require.Equal(t, "Done", clie2e.DryRunGet(result.Stdout, "api.0.body.operations.0.name").String())
	require.Equal(t, "Doing", clie2e.DryRunGet(result.Stdout, "api.0.body.operations.1.new_name").String())
	require.Equal(t, "delete", clie2e.DryRunGet(result.Stdout, "api.0.body.operations.2.operation").String())
}
