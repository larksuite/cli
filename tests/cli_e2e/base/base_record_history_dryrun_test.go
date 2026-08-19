// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseRecordHistoryListDryRunUsesExplicitRecordID(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+record-history-list",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--record-id", "rec_selected",
		"--page-size", "10",
	)

	out := result.Stdout
	require.Equal(t, "GET", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/record_history", gjson.Get(out, "data.api.0.url").String(), out)
	require.Equal(t, "app_x", gjson.Get(out, "data.base_token").String(), out)
	require.Equal(t, "tbl_x", gjson.Get(out, "data.api.0.params.table_id").String(), out)
	require.Equal(t, "rec_selected", gjson.Get(out, "data.api.0.params.record_id").String(), out)
	require.Equal(t, int64(10), gjson.Get(out, "data.api.0.params.page_size").Int(), out)
}

func TestBaseRecordHistoryListDryRunRejectsNonPositiveMaxVersion(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		t.Run(value, func(t *testing.T) {
			result := runBaseDryRun(t, 2,
				"base", "+record-history-list",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--record-id", "rec_selected",
				"--max-version", value,
			)

			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--max-version", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "must be greater than 0")
			require.Empty(t, result.Stdout)
		})
	}
}
