// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseViewCreateDryRun(t *testing.T) {
	t.Run("create view", func(t *testing.T) {
		result := runBaseDryRun(t, 0,
			"base", "+view-create",
			"--base-token", "app_x",
			"--table-id", "tbl_new",
			"--json", `{"name":"New view","type":"grid"}`,
		)

		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_new/views", clie2e.DryRunGet(out, "api.0.url").String(), out)
		require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
		require.Equal(t, "New view", clie2e.DryRunGet(out, "api.0.body.name").String(), out)
		require.Equal(t, "grid", clie2e.DryRunGet(out, "api.0.body.type").String(), out)
	})

	t.Run("reject empty batch", func(t *testing.T) {
		result := runBaseDryRun(t, 2,
			"base", "+view-create",
			"--base-token", "app_x",
			"--table-id", "tbl_new",
			"--json", `[]`,
		)

		require.Empty(t, result.Stdout)
		require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
		require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
		require.Equal(t, "--json", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
		require.Contains(t, result.Stderr, "at least one view")
	})
}
