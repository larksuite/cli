// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseTableCreateDryRunIncludesViews(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+table-create",
		"--base-token", "app_x",
		"--name", "Tasks",
		"--view", `[{"name":"First","type":"grid"},{"name":"Second","type":"grid"}]`,
	)

	require.Equal(t, int64(3), gjson.Get(result.Stdout, "data.api.#").Int(), result.Stdout)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables", gjson.Get(result.Stdout, "data.api.0.url").String(), result.Stdout)
	for index, name := range []string{"First", "Second"} {
		path := "data.api." + strconv.Itoa(index+1)
		require.Equal(t, "POST", gjson.Get(result.Stdout, path+".method").String(), result.Stdout)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/%3Ccreated_table_id%3E/views", gjson.Get(result.Stdout, path+".url").String(), result.Stdout)
		require.Equal(t, name, gjson.Get(result.Stdout, path+".body.name").String(), result.Stdout)
	}
}

func TestBaseTableCreateDryRunRejectsInvalidView(t *testing.T) {
	result := runBaseDryRun(t, 2,
		"base", "+table-create",
		"--base-token", "app_x",
		"--name", "Tasks",
		"--view", `[1]`,
	)

	require.Empty(t, result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--view", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "--view item 1 must be an object")
}
