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

func TestBaseFieldCreateDryRunButtonNormalizesTriggerPayload(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+field-create",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--json", `{"name":"同步到 CRM","type":"button","button":{"title":"同步到 CRM","color":0},"trigger":{"type":"automation","workflow_id":"wkf_x"}}`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "同步到 CRM", clie2e.DryRunGet(out, "api.0.body.name").String(), out)
	require.Equal(t, int64(3001), clie2e.DryRunGet(out, "api.0.body.type").Int(), out)
	require.Equal(t, "Button", clie2e.DryRunGet(out, "api.0.body.fieldUIType").String(), out)
	require.Equal(t, "同步到 CRM", clie2e.DryRunGet(out, "api.0.body.property.button.title").String(), out)
	require.Equal(t, int64(0), clie2e.DryRunGet(out, "api.0.body.property.button.color").Int(), out)
	require.Equal(t, int64(1), clie2e.DryRunGet(out, "api.0.body.property.trigger.type").Int(), out)
	require.Equal(t, "wkf_x", clie2e.DryRunGet(out, "api.0.body.property.trigger.config.id").String(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.button").Exists(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.trigger").Exists(), out)
}
