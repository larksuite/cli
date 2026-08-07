// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseFieldUpdateAutoNumberDryRun(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+field-update",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--field-id", "fld_x",
		"--json", `{"name":"编号","type":"auto_number","style":{"rules":[{"type":"text","text":"TASK-"},{"type":"created_time","date_format":"yyyyMM"},{"type":"text","text":"-"},{"type":"incremental_number","length":4}]}}`,
		"--yes",
	)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x", gjson.Get(out, "data.api.0.url").String(), out)
	require.Equal(t, "PUT", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "编号", gjson.Get(out, "data.api.0.body.name").String(), out)
	require.Equal(t, "auto_number", gjson.Get(out, "data.api.0.body.type").String(), out)
	require.Equal(t, "created_time", gjson.Get(out, "data.api.0.body.style.rules.1.type").String(), out)
	require.Equal(t, "yyyyMM", gjson.Get(out, "data.api.0.body.style.rules.1.date_format").String(), out)
	require.Equal(t, int64(4), gjson.Get(out, "data.api.0.body.style.rules.3.length").Int(), out)
	require.False(t, gjson.Get(out, "data.api.0.body.property.auto_serial").Exists(), out)
	require.NotContains(t, out, "reformat_existing_records", out)
	require.NotContains(t, out, "/open-apis/bitable/v1/", out)
}

func TestBaseFieldUpdateAutoNumberReformatDryRun(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+field-update",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--field-id", "fld_x",
		"--json", `{"name":"编号","type":"auto_number","style":{"rules":[{"type":"text","text":"TASK-"},{"type":"created_time","date_format":"yyyyMM"},{"type":"text","text":"-"},{"type":"incremental_number","length":4}]}}`,
		"--reformat-existing-records",
		"--yes",
	)

	out := result.Stdout
	require.Equal(t, "/open-apis/bitable/v1/apps/app_x/tables/tbl_x/fields/fld_x", gjson.Get(out, "data.api.0.url").String(), out)
	require.Equal(t, "PUT", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "编号", gjson.Get(out, "data.api.0.body.field_name").String(), out)
	require.Equal(t, int64(1005), gjson.Get(out, "data.api.0.body.type").Int(), out)
	require.True(t, gjson.Get(out, "data.api.0.body.property.auto_serial.reformat_existing_records").Bool(), out)
	require.Equal(t, "fixed_text", gjson.Get(out, "data.api.0.body.property.auto_serial.options.0.type").String(), out)
	require.Equal(t, "created_time", gjson.Get(out, "data.api.0.body.property.auto_serial.options.1.type").String(), out)
	require.Equal(t, "system_number", gjson.Get(out, "data.api.0.body.property.auto_serial.options.3.type").String(), out)
	require.Equal(t, "4", gjson.Get(out, "data.api.0.body.property.auto_serial.options.3.value").String(), out)
}

func TestBaseFieldUpdateDryRunRejectsRatingMaxAboveLimit(t *testing.T) {
	result := runBaseDryRun(t, 2,
		"base", "+field-update",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--field-id", "fld_x",
		"--json", `{"name":"评分","type":"number","style":{"type":"rating","icon":"star","min":0,"max":20}}`,
		"--yes",
	)

	require.Empty(t, strings.TrimSpace(result.Stdout), result.Stdout)
	errMsg := gjson.Get(result.Stderr, "error.message").String()
	for _, want := range []string{"--json.style.max", "rating max must be <= 10", "plain number/progress field"} {
		if !strings.Contains(errMsg, want) {
			t.Fatalf("expected %q in dry-run error, got %q\nstderr:\n%s", want, errMsg, result.Stderr)
		}
	}
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "--json", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
}
