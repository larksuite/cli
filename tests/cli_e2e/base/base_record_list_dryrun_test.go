// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseRecordListDryRunAcceptsFieldsAlias(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+record-list",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--fields", `["Name","Age"]`,
		"--limit", "3",
	)

	out := result.Stdout
	require.Equal(t, "GET", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records?field_id=Name&field_id=Age&limit=3&offset=0", gjson.Get(out, "data.api.0.url").String(), out)
}

func TestBaseRecordSearchDryRunAcceptsFieldsAlias(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+record-search",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--keyword", "Alice",
		"--search-field", "Name",
		"--fields", `["Name","Age"]`,
	)

	out := result.Stdout
	require.Equal(t, "POST", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search", gjson.Get(out, "data.api.0.url").String(), out)
	require.Equal(t, "Name", gjson.Get(out, "data.api.0.body.select_fields.0").String(), out)
	require.Equal(t, "Age", gjson.Get(out, "data.api.0.body.select_fields.1").String(), out)
}

func TestBaseRecordGetDryRunAcceptsFieldNamesAlias(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+record-get",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--record-id", "rec_1",
		"--field-names", "Name",
		"--field-names", "Age",
	)

	out := result.Stdout
	require.Equal(t, "POST", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get", gjson.Get(out, "data.api.0.url").String(), out)
	require.Equal(t, "rec_1", gjson.Get(out, "data.api.0.body.record_id_list.0").String(), out)
	require.Equal(t, "Name", gjson.Get(out, "data.api.0.body.select_fields.0").String(), out)
	require.Equal(t, "Age", gjson.Get(out, "data.api.0.body.select_fields.1").String(), out)
}

func TestBaseRecordGetDryRunTreatsNullProjectionAsOmitted(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+record-get",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--json", `{"record_id_list":["rec_1"],"select_fields":null}`,
	)

	out := result.Stdout
	require.Equal(t, "POST", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get", gjson.Get(out, "data.api.0.url").String(), out)
	require.Equal(t, "rec_1", gjson.Get(out, "data.api.0.body.record_id_list.0").String(), out)
	require.False(t, gjson.Get(out, "data.api.0.body.select_fields").Exists(), out)
}

func TestBaseRecordGetDryRunUsesFlagProjectionWhenJSONProjectionIsNull(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+record-get",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--json", `{"record_id_list":["rec_1"],"select_fields":null}`,
		"--field-id", "Name",
	)

	out := result.Stdout
	require.Equal(t, "POST", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get", gjson.Get(out, "data.api.0.url").String(), out)
	require.Equal(t, "rec_1", gjson.Get(out, "data.api.0.body.record_id_list.0").String(), out)
	require.Equal(t, "Name", gjson.Get(out, "data.api.0.body.select_fields.0").String(), out)
}

func TestBaseRecordListDryRunPreservesFieldNamesCSVSemantics(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+record-list",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--field-names", `"A,B",@Owner`,
		"--limit", "3",
	)

	out := result.Stdout
	require.Equal(t, "GET", gjson.Get(out, "data.api.0.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records?field_id=A%2CB&field_id=%40Owner&limit=3&offset=0", gjson.Get(out, "data.api.0.url").String(), out)
}

func TestBaseRecordListDryRunTreatsLeadingAtFieldNameLiterally(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+record-list",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--field-names", "@Owner",
		"--limit", "3",
	)
	require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/records?field_id=%40Owner&limit=3&offset=0", gjson.Get(result.Stdout, "data.api.0.url").String(), result.Stdout)
}

func TestBaseRecordSearchDryRunJSONConflictReportsActualParams(t *testing.T) {
	result := runBaseDryRun(t, 2,
		"base", "+record-search",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--json", `{"keyword":"Alice","search_fields":["Name"]}`,
		"--field-names", "Age",
	)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--json", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Equal(t, int64(2), gjson.Get(result.Stderr, "error.params.#").Int(), result.Stderr)
	require.Equal(t, "--json", gjson.Get(result.Stderr, "error.params.0.name").String(), result.Stderr)
	require.Equal(t, "--field-names", gjson.Get(result.Stderr, "error.params.1.name").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "inside --json")
	require.Empty(t, result.Stdout)
}

func TestBaseRecordProjectionDryRunKeepsActiveParamForFlagLikeFieldNames(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantParam string
	}{
		{
			name: "canonical",
			args: []string{
				"base", "+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
				"--field-id", "Cost--USD", "--field-id", "Cost--USD",
			},
			wantParam: "--field-id",
		},
		{
			name: "fields alias",
			args: []string{
				"base", "+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
				"--fields", `["Cost--USD","Cost--USD"]`,
			},
			wantParam: "--fields",
		},
		{
			name: "field names alias",
			args: []string{
				"base", "+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
				"--field-names", "Cost--USD", "--field-names", "Cost--USD",
			},
			wantParam: "--field-names",
		},
		{
			name: "json projection",
			args: []string{
				"base", "+record-search", "--base-token", "app_x", "--table-id", "tbl_x",
				"--json", `{"keyword":"cost","search_fields":["Name"],"select_fields":["Cost--USD","Cost--USD"]}`,
			},
			wantParam: "--json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runBaseDryRun(t, 2, tc.args...)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, tc.wantParam, gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Equal(t, int64(1), gjson.Get(result.Stderr, "error.params.#").Int(), result.Stderr)
			require.Equal(t, tc.wantParam, gjson.Get(result.Stderr, "error.params.0.name").String(), result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "duplicate field id")
			require.Empty(t, result.Stdout)
		})
	}
}
