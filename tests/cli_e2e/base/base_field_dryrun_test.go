// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseFieldGetDryRunAcceptsFieldIDOrNameAlias(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+field-get",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--field-id-or-name", "Amount",
	)

	out := result.Stdout
	require.Equal(t, "GET", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/Amount", gjson.Get(out, "data.api.0.url").String(), out)
	require.Equal(t, "Amount", gjson.Get(out, "data.field_id").String(), out)
}

func TestBaseFieldGetDryRunRequiresFieldSelectorBeforeRun(t *testing.T) {
	result := runBaseDryRun(t, 2,
		"base", "+field-get",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
	)
	require.Empty(t, strings.TrimSpace(result.Stdout), result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "--field-id", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "--field-id is required")
}

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
