// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const progressTestCustom = `{"type":"custom","value":0}`
const progressTestField = `{"type":"field","table_name":"Orders","count_all":true}`
const progressTestToday = `{"type":"relative","value":"today"}`
const progressTestMonth = `{"type":"relative","value":"month"}`

func progressTestConfig(mode, current, target string) string {
	return `{"progress":{"type":"` + mode + `","current":` + current + `,"target":` + target + `}}`
}

func TestDashboardProgressCreate(t *testing.T) {
	tests := []struct{ name, config string }{
		{"zero and negative fraction", progressTestConfig("value", progressTestCustom, `{"type":"custom","value":-0.25}`)},
		{"record count", progressTestConfig("value", progressTestField, progressTestCustom)},
		{"target field", progressTestConfig("value", progressTestCustom, progressTestField)},
		{"independent tables", progressTestConfig("value", progressTestField, `{"type":"field","table_name":"Goals","series":[{"field_name":"Amount","rollup":"AVERAGE"}],"filter":{"conjunction":"or","conditions":[{"field_name":"Name","operator":"is","value":["kept"]}]}}`)},
		{"relative dates", progressTestConfig("date", progressTestToday, progressTestMonth)},
		{"custom dates", progressTestConfig("date", `{"type":"custom","value":0}`, `{"type":"custom","range":[-9007199254740991,9007199254740991]}`)},
		{"mixed dates", progressTestConfig("date", `{"type":"custom","value":1790000000000}`, `{"type":"relative","value":"year"}`)},
		{"custom format", `{"progress":{"type":"date","current":{"type":"relative","value":"yesterday"},"target":{"type":"custom","range":[0,1]},"dateFormat":"DD/MM/YYYY"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: "POST", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks",
				RawBody: []byte(`{"code":0,"data":{"block_id":"blk_progress","type":"progress","data_config":` + tc.config + `}}`)}
			reg.Register(stub)
			err := runShortcut(t, BaseDashboardBlockCreate, []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Progress", "--type", "progress", "--data-config", tc.config}, factory, stdout)
			require.NoError(t, err)
			require.JSONEq(t, tc.config, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
			require.Equal(t, "progress", gjson.GetBytes(stub.CapturedBody, "type").String())
			require.True(t, gjson.Get(stdout.String(), "data.created").Bool(), stdout.String())
			require.Equal(t, "blk_progress", gjson.Get(stdout.String(), "data.block.block_id").String())
			require.JSONEq(t, tc.config, gjson.Get(stdout.String(), "data.block.data_config").Raw)
		})
	}
}

func TestDashboardProgressInvalidCreate(t *testing.T) {
	valueWith := func(source string) string { return progressTestConfig("value", source, progressTestCustom) }
	dateWith := func(current, target string) string { return progressTestConfig("date", current, target) }
	tests := []struct{ name, config, want string }{
		{"missing config", "", "progress.type/current/target"},
		{"empty", `{}`, "progress"},
		{"null progress", `{"progress":null}`, "progress"},
		{"number alias", progressTestConfig("number", progressTestCustom, progressTestCustom), "progress.type"},
		{"missing current", `{"progress":{"type":"value","target":{"type":"custom","value":1}}}`, "progress.current"},
		{"missing target", `{"progress":{"type":"value","current":{"type":"custom","value":1}}}`, "progress.target"},
		{"unknown wrapper", `{"progress":{"type":"value","current":{"type":"custom","value":0},"target":{"type":"custom","value":1}},"table_name":"Orders"}`, "不支持字段 table_name"},
		{"missing source type", valueWith(`{"value":0}`), "progress.current.type"},
		{"null custom", valueWith(`{"type":"custom","value":null}`), "value"},
		{"string custom", valueWith(`{"type":"custom","value":"100"}`), "有限数字"},
		{"cross source fields", valueWith(`{"type":"custom","value":0,"filter":null}`), "不支持字段 filter"},
		{"missing table", valueWith(`{"type":"field","count_all":true}`), "table_name"},
		{"missing metric", valueWith(`{"type":"field","table_name":"Orders"}`), "count_all:true 或 series"},
		{"false count", valueWith(`{"type":"field","table_name":"Orders","count_all":false}`), "count_all"},
		{"null count", valueWith(`{"type":"field","table_name":"Orders","count_all":null}`), "count_all"},
		{"both metrics", valueWith(`{"type":"field","table_name":"Orders","count_all":true,"series":[{"field_name":"Amount","rollup":"SUM"}]}`), "必须且只能"},
		{"empty series", valueWith(`{"type":"field","table_name":"Orders","series":[]}`), "恰好包含 1"},
		{"two series", valueWith(`{"type":"field","table_name":"Orders","series":[{"field_name":"A","rollup":"MIN"},{"field_name":"B","rollup":"MAX"}]}`), "恰好包含 1"},
		{"null series item", valueWith(`{"type":"field","table_name":"Orders","series":[null]}`), "series[0]"},
		{"extra series field", valueWith(`{"type":"field","table_name":"Orders","series":[{"field_name":"A","rollup":"SUM","field_id":"fld_x"}]}`), "不支持字段 field_id"},
		{"lowercase rollup", valueWith(`{"type":"field","table_name":"Orders","series":[{"field_name":"A","rollup":"sum"}]}`), "rollup"},
		{"COUNTA rollup", valueWith(`{"type":"field","table_name":"Orders","series":[{"field_name":"A","rollup":"COUNTA"}]}`), "rollup"},
		{"groups", valueWith(`{"type":"field","table_name":"Orders","count_all":true,"group_by":[]}`), "不支持字段 group_by"},
		{"null filter", valueWith(`{"type":"field","table_name":"Orders","count_all":true,"filter":null}`), "filter 必须是对象"},
		{"empty filter", valueWith(`{"type":"field","table_name":"Orders","count_all":true,"filter":{"conjunction":"and","conditions":[]}}`), "conditions"},
		{"nested filter", valueWith(`{"type":"field","table_name":"Orders","count_all":true,"filter":{"conjunction":"and","conditions":[{"field_name":"A","operator":"isEmpty"}],"children":[]}}`), "不支持字段 children"},
		{"extra condition", valueWith(`{"type":"field","table_name":"Orders","count_all":true,"filter":{"conjunction":"and","conditions":[{"field_name":"A","operator":"isEmpty","extra":true}]}}`), "不支持字段 extra"},
		{"date field source", dateWith(progressTestField, progressTestMonth), "日期模式不支持 field"},
		{"relative current enum", dateWith(progressTestMonth, progressTestMonth), "today|yesterday"},
		{"relative target enum", dateWith(progressTestToday, progressTestToday), "month|quarter|year"},
		{"fractional timestamp", dateWith(`{"type":"custom","value":0.5}`, progressTestMonth), "安全整数"},
		{"unsafe timestamp", dateWith(`{"type":"custom","value":9007199254740992}`, progressTestMonth), "安全整数"},
		{"half range", dateWith(progressTestToday, `{"type":"custom","range":[0]}`), "恰好包含 2"},
		{"equal range", dateWith(progressTestToday, `{"type":"custom","range":[0,0]}`), "起点必须小于终点"},
		{"reverse range", dateWith(progressTestToday, `{"type":"custom","range":[2,1]}`), "起点必须小于终点"},
		{"mixed range and relative", dateWith(progressTestToday, `{"type":"relative","value":"month","range":[0,1]}`), "不支持字段 range"},
		{"null format", `{"progress":{"type":"date","current":{"type":"relative","value":"today"},"target":{"type":"relative","value":"month"},"dateFormat":null}}`, "dateFormat"},
		{"date format in value mode", `{"progress":{"type":"value","current":{"type":"custom","value":0},"target":{"type":"custom","value":1},"dateFormat":"YYYY-MM-DD"}}`, "不支持字段 dateFormat"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: "POST", URL: "/blocks", Optional: true, RawBody: []byte(`{"code":0}`)}
			reg.Register(stub)
			args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Invalid", "--type", "progress"}
			if tc.config != "" {
				args = append(args, "--data-config", tc.config)
			}
			err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Equal(t, "--data-config", validation.Param)
			require.Contains(t, validation.Error(), tc.want)
			require.Empty(t, stub.CapturedBodies)
		})
	}
}

func TestDashboardProgressFilterLimits(t *testing.T) {
	for _, tc := range []struct {
		conditions, values int
		valid              bool
	}{{1, 200, true}, {50, 1, true}, {1, 201, false}, {51, 1, false}} {
		values := make([]interface{}, tc.values)
		for i := range values {
			values[i] = "option"
		}
		conditions := make([]interface{}, tc.conditions)
		for i := range conditions {
			conditions[i] = map[string]interface{}{"field_name": "Status", "operator": "is", "value": values}
		}
		cfg := map[string]interface{}{"progress": map[string]interface{}{"type": "value", "current": map[string]interface{}{"type": "field", "table_name": "Orders", "count_all": true, "filter": map[string]interface{}{"conjunction": "and", "conditions": conditions}}, "target": map[string]interface{}{"type": "custom", "value": float64(100)}}}
		require.Equal(t, tc.valid, len(validateBlockDataConfig("progress", cfg)) == 0)
	}
}

func TestDashboardProgressUpdatePreservesPatch(t *testing.T) {
	for _, patch := range []string{
		`{}`, `{"progress":{}}`, `{"progress":{"current":{}}}`,
		`{"progress":{"target":{"value":0}}}`,
		`{"progress":{"current":{"filter":null}}}`,
		`{"progress":{"current":{"series":[{"field_name":"Amount","rollup":"MAX"}]}}}`,
		`{"progress":{"dateFormat":"MM-DD"}}`,
		`{"progress":{"target":{"type":"relative","value":"quarter"}}}`,
		progressTestConfig("date", progressTestToday, progressTestMonth),
	} {
		t.Run(patch, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			full := progressTestConfig("value", progressTestField, progressTestCustom)
			stub := &httpmock.Stub{Method: "PATCH", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x","data_config":` + full + `}}`)}
			reg.Register(stub)
			require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x", "--data-config", patch}, factory, stdout))
			require.JSONEq(t, patch, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
			require.False(t, gjson.GetBytes(stub.CapturedBody, "type").Exists())
			require.True(t, gjson.Get(stdout.String(), "data.updated").Bool())
			require.JSONEq(t, full, gjson.Get(stdout.String(), "data.block.data_config").Raw)
		})
	}
}

func TestDashboardProgressGetPreservesConfig(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	full := `{"progress":{"type":"date","current":{"type":"relative","value":"today"},"target":{"type":"relative","value":"month"},"dateFormat":"YYYY-MM-DD"}}`
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x","type":"progress","data_config":` + full + `}}`)})
	require.NoError(t, runShortcut(t, BaseDashboardBlockGet, []string{"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x"}, factory, stdout))
	require.JSONEq(t, full, gjson.Get(stdout.String(), "data.block.data_config").Raw)
}

func TestDashboardProgressFileAndNoValidate(t *testing.T) {
	for _, tc := range []struct {
		name, method, command, config string
		shortcut                      common.Shortcut
		extra                         []string
	}{
		{"create file", "POST", "+dashboard-block-create", progressTestConfig("date", progressTestToday, progressTestMonth), BaseDashboardBlockCreate, []string{"--name", "Progress", "--type", "progress"}},
		{"update file", "PATCH", "+dashboard-block-update", `{"progress":{"current":{"filter":null}}}`, BaseDashboardBlockUpdate, []string{"--block-id", "blk_x"}},
		{"no validate", "POST", "+dashboard-block-create", `{"progress":{"type":"unknown","value":false}}`, BaseDashboardBlockCreate, []string{"--name", "Progress", "--type", "progress", "--no-validate"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			withBaseWorkingDir(t, dir)
			path := filepath.Join(dir, "config.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.config), 0600))
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: tc.method, URL: "/blocks", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x"}}`)}
			reg.Register(stub)
			args := append([]string{tc.command, "--base-token", "app_x", "--dashboard-id", "dsh_x", "--data-config", "@" + path}, tc.extra...)
			require.NoError(t, runShortcut(t, tc.shortcut, args, factory, stdout))
			require.JSONEq(t, tc.config, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
		})
	}
	// Adding a dashboard type must not expand the shared BaseApp type list.
	require.False(t, isAppBlockType("progress"))
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(progressTestConfig("date", progressTestToday, progressTestMonth)), &cfg))
	normalized := normalizeDataConfigForCreate("progress", cfg)
	require.Equal(t, cfg, normalized)
}
