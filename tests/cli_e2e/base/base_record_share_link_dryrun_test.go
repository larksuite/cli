// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseRecordShareLinkCreateDryRunAcceptsSingularAndPluralRecordIDFlags(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+record-share-link-create",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--record-id", "rec_1",
		"--record-ids", "rec_2,rec_3",
	)

	out := result.Stdout
	require.Equal(t, "POST", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/share_links/batch", gjson.Get(out, "data.api.0.url").String(), out)
	require.Equal(t, []string{"rec_1", "rec_2", "rec_3"}, []string{
		gjson.Get(out, "data.api.0.body.record_ids.0").String(),
		gjson.Get(out, "data.api.0.body.record_ids.1").String(),
		gjson.Get(out, "data.api.0.body.record_ids.2").String(),
	}, out)
}
