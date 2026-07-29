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

func TestBaseRecordBatchUpdatePerRecordDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+record-batch-update",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--json", `{"update_records":{"recA":{"Status":["Done"]},"recB":{"Score":20}}}`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_update", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "Done", clie2e.DryRunGet(out, "api.0.body.update_records.recA.Status.0").String(), out)
	require.Equal(t, int64(20), clie2e.DryRunGet(out, "api.0.body.update_records.recB.Score").Int(), out)
}
