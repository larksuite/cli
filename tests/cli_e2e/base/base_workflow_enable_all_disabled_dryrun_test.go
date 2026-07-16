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

func TestBaseWorkflowEnableAllDisabledDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+workflow-enable-all-disabled",
			"--base-token", "app_x",
			"--page-size", "50",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/workflows/list", gjson.Get(out, "api.0.url").String(), out)
	require.Equal(t, int64(50), gjson.Get(out, "api.0.body.page_size").Int(), out)
	require.Equal(t, "disabled", gjson.Get(out, "api.0.body.status").String(), out)
	require.Equal(t, "PATCH", gjson.Get(out, "api.1.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/workflows/%3Cworkflow_id%3E/enable", gjson.Get(out, "api.1.url").String(), out)
}
