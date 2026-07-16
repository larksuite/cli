// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseDashboardBlockBatchCreateDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	blocks := `[{"name":"订单数","type":"statistics","data_config":{"table_name":"订单表","count_all":true}},{"name":"月度趋势","type":"line","data_config":{"table_name":"订单表","series":[{"field_name":"金额","rollup":"SUM"}],"group_by":[{"field_name":"月份","mode":"integrated","sort":{"type":"group","order":"asc"}}]}}]`
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-block-batch-create",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
			"--blocks", blocks,
			"--user-id-type", "open_id",
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks")
	assert.Contains(t, output, `"method": "POST"`)
	assert.Contains(t, output, `"base_token": "app_x"`)
	assert.Contains(t, output, `"dashboard_id": "dsh_1"`)
	assert.Contains(t, output, `"user_id_type": "open_id"`)
	assert.Contains(t, output, `"blocks"`)
	assert.Contains(t, output, `"sequential": true`)
	assert.Contains(t, output, `"name": "订单数"`)
	assert.Contains(t, output, `"type": "line"`)
}
