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

func TestBaseRecordListDryRunAcceptsJSONOutputAlias(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+record-list",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--limit", "5",
			"--field-id", "Name",
			"--json",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	url := gjson.Get(result.Stdout, "api.0.url").String()
	require.Contains(t, url, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records")
	require.Contains(t, url, "limit=5")
	require.Contains(t, url, "field_id=Name")
}
