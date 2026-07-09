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
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields", gjson.Get(out, "api.0.url").String(), out)
	require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), out)
	require.Equal(t, "A", gjson.Get(out, "api.0.body.name").String(), out)
	require.Equal(t, "text", gjson.Get(out, "api.0.body.type").String(), out)

	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields", gjson.Get(out, "api.1.url").String(), out)
	require.Equal(t, "POST", gjson.Get(out, "api.1.method").String(), out)
	require.Equal(t, "B", gjson.Get(out, "api.1.body.name").String(), out)
	require.Equal(t, "text", gjson.Get(out, "api.1.body.type").String(), out)
}
