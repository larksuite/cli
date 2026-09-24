// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The raw API is the documented entry until the gateway publishes the service
// metadata. These checks validate its wire request without live credentials.
func TestDashboardAdvPermAPIDryRun(t *testing.T) {
	const path = "/open-apis/base/v3/bases/base_test/dashboards/blk_test/adv_perm"
	get := runBaseDryRun(t, 0, "api", "GET", path)
	require.Equal(t, "GET", gjson.Get(get.Stdout, "data.api.0.method").String())
	require.Equal(t, path, gjson.Get(get.Stdout, "data.api.0.url").String())
	require.False(t, gjson.Get(get.Stdout, "data.api.0.body").Exists(), get.Stdout)

	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			result := runBaseDryRun(t, 0, "api", "PATCH", path,
				"--data", fmt.Sprintf(`{"is_adv_perm_enable":%t}`, enabled))
			require.Equal(t, "PATCH", gjson.Get(result.Stdout, "data.api.0.method").String())
			require.Equal(t, path, gjson.Get(result.Stdout, "data.api.0.url").String())
			state := gjson.Get(result.Stdout, "data.api.0.body.is_adv_perm_enable")
			require.True(t, state.Exists(), result.Stdout)
			require.Equal(t, fmt.Sprint(enabled), state.Raw)
		})
	}
}
