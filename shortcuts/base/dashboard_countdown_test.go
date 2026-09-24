// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const countdownFieldConfig = `{"type":"field","aggregateType":"MIN","additionalDatasources":[{"tableName":"Tasks","countAll":true,"groupBy":[{"fieldName":"Deadline"}]}]}`

func TestDashboardCountdownCreateAndGet(t *testing.T) {
	for _, config := range []string{
		`{"type":"fixed","targetTime":0}`,
		`{"type":"fixed","targetTime":1798732800000}`,
		countdownFieldConfig,
		`{"type":"field","aggregateType":"MAX","additionalDatasources":[{"tableName":"Tasks","series":[{"fieldName":"Cost","rollup":"SUM"}],"groupBy":[{"fieldName":"Deadline","mode":"enumerated","sort":{"type":"view"}}],"filter":{"conjunction":"and","conditions":[{"fieldName":"Cost","operator":"is","value":0},{"fieldName":"Done","operator":"is","value":false},{"fieldName":"Deadline","operator":"is","value":["ExactDate",1798732800000]}]}}]}`,
	} {
		t.Run(config, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			// The server can normalize readback without changing the request.
			readback := strings.ReplaceAll(config, `"fieldName":"Deadline"}`, `"fieldName":"Deadline","mode":"integrated"}`)
			block := `{"block_id":"blk_countdown","name":"Deadline","type":"countdown","position":{"x":0,"y":0,"w":4,"h":4},"data_config":` + readback + `}`
			response := []byte(`{"code":0,"data":` + block + `}`)
			create := &httpmock.Stub{Method: "POST", URL: "/bases/app_x/dashboards/dsh_x/blocks", RawBody: response}
			reg.Register(create)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "/bases/app_x/dashboards/dsh_x/blocks/blk_countdown", RawBody: response})
			err := runShortcut(t, BaseDashboardBlockCreate, []string{
				"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Deadline", "--type", "countdown", "--data-config", config,
			}, factory, stdout)
			require.NoError(t, err)
			require.JSONEq(t, config, gjson.GetBytes(create.CapturedBody, "data_config").Raw)
			require.Equal(t, "countdown", gjson.GetBytes(create.CapturedBody, "type").String())
			require.True(t, gjson.Get(stdout.String(), "data.created").Bool())
			require.JSONEq(t, block, gjson.Get(stdout.String(), "data.block").Raw)
			err = runShortcut(t, BaseDashboardBlockGet, []string{
				"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_countdown",
			}, factory, stdout)
			require.NoError(t, err)
			require.JSONEq(t, block, gjson.Get(stdout.String(), "data.block").Raw)
		})
	}
}

// These assertions cover CLI forwarding, not a simulated implementation of
// server-side merging. A complete server response must survive even for {}.
func TestDashboardCountdownUpdatePreservesPatch(t *testing.T) {
	for _, patch := range []string{
		`{}`, `{"targetTime":0}`, `{"aggregateType":"MAX"}`,
		`{"type":"fixed"}`, // Valid only when already fixed; the server decides.
		`{"type":"fixed","targetTime":1798732800000}`,
		countdownFieldConfig,
		`{"additionalDatasources":[{}]}`,
		`{"additionalDatasources":[{"filter":null}]}`,
		`{"additionalDatasources":[{"filter":{"conjunction":"and","conditions":[{"fieldName":"Done","operator":"is","value":false}]}}]}`,
		`{"additionalDatasources":[{"groupBy":[{"fieldName":"Created","sort":{"type":"group"}}]}]}`,
		`{"additionalDatasources":[{"series":[{"fieldName":"Cost","rollup":"SUM"}]}]}`,
		`{"additionalDatasources":[{"countAll":true}]}`,
		`{"additionalDatasources":[{"tableName":"Milestones","countAll":true,"groupBy":[{"fieldName":"Launch"}]}]}`,
	} {
		t.Run(patch, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{
				Method: "PATCH", URL: "/bases/app_x/dashboards/dsh_x/blocks/blk_countdown",
				RawBody: []byte(`{"code":0,"data":{"block_id":"blk_countdown","type":"countdown","data_config":` + countdownFieldConfig + `}}`),
			}
			reg.Register(stub)
			err := runShortcut(t, BaseDashboardBlockUpdate, []string{
				"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_countdown", "--data-config", patch,
			}, factory, stdout)
			require.NoError(t, err)
			require.JSONEq(t, `{"data_config":`+patch+`}`, string(stub.CapturedBody))
			require.True(t, gjson.Get(stdout.String(), "data.updated").Bool())
			require.JSONEq(t, countdownFieldConfig, gjson.Get(stdout.String(), "data.block.data_config").Raw)
		})
	}
}

func TestDashboardCountdownCreateRejectsInvalidConfig(t *testing.T) {
	fieldWith := func(source string) string {
		return `{"type":"field","aggregateType":"MIN","additionalDatasources":[` + source + `]}`
	}
	for _, tc := range []struct{ name, config, message string }{
		{"missing config", "", "必须提供 data-config"},
		{"missing mode", `{}`, "countdown.type"},
		{"invalid mode", `{"type":"FIXED","targetTime":0}`, "countdown.type"},
		{"missing target", `{"type":"fixed"}`, "targetTime"},
		{"null target", `{"type":"fixed","targetTime":null}`, "targetTime"},
		{"string target", `{"type":"fixed","targetTime":"0"}`, "targetTime"},
		{"negative target", `{"type":"fixed","targetTime":-1}`, "targetTime"},
		{"fractional target", `{"type":"fixed","targetTime":0.5}`, "targetTime"},
		{"mixed modes", `{"type":"fixed","targetTime":0,"aggregateType":null}`, "aggregateType"},
		{"hidden display option", `{"type":"fixed","targetTime":0,"units":["day"]}`, "units"},
		{"snake case", `{"type":"fixed","target_time":0}`, "target_time"},
		{"invalid aggregate", strings.Replace(countdownFieldConfig, `"MIN"`, `"min"`, 1), "aggregateType"},
		{"multiple sources", `{"type":"field","aggregateType":"MIN","additionalDatasources":[{},{}]}`, "1 个数据源"},
		{"null source", fieldWith(`null`), "必须是对象"},
		{"missing metric", fieldWith(`{"tableName":"Tasks","groupBy":[{"fieldName":"Deadline"}]}`), "二选一"},
		{"both metrics", strings.Replace(countdownFieldConfig, `"countAll":true`, `"countAll":true,"series":[{"fieldName":"Cost","rollup":"SUM"}]`, 1), "二选一"},
		{"false count", strings.Replace(countdownFieldConfig, `"countAll":true`, `"countAll":false`, 1), "只能为 true"},
		{"null count", strings.Replace(countdownFieldConfig, `"countAll":true`, `"countAll":null`, 1), "只能为 true"},
		{"empty series", strings.Replace(countdownFieldConfig, `"countAll":true`, `"series":[]`, 1), "1..20"},
		{"invalid rollup", strings.Replace(countdownFieldConfig, `"countAll":true`, `"series":[{"fieldName":"Cost","rollup":"sum"}]`, 1), "rollup"},
		{"blank table", strings.Replace(countdownFieldConfig, `"Tasks"`, `" "`, 1), "tableName"},
		{"extra source field", strings.Replace(countdownFieldConfig, `"tableName":"Tasks"`, `"tableName":"Tasks","table_id":"tbl_x"`, 1), "table_id"},
		{"two dates", strings.Replace(countdownFieldConfig, `{"fieldName":"Deadline"}`, `{"fieldName":"Deadline"},{"fieldName":"Created"}`, 1), "1 个日期字段"},
		{"null mode", strings.Replace(countdownFieldConfig, `{"fieldName":"Deadline"}`, `{"fieldName":"Deadline","mode":null}`, 1), "mode"},
		{"null sort", strings.Replace(countdownFieldConfig, `{"fieldName":"Deadline"}`, `{"fieldName":"Deadline","sort":null}`, 1), "sort"},
		{"bad sort order", strings.Replace(countdownFieldConfig, `{"fieldName":"Deadline"}`, `{"fieldName":"Deadline","sort":{"type":"group","order":"ASC"}}`, 1), "sort.order"},
		{"null filter", fieldWith(`{"tableName":"Tasks","countAll":true,"groupBy":[{"fieldName":"Deadline"}],"filter":null}`), "filter:null"},
		{"empty filter", fieldWith(`{"tableName":"Tasks","countAll":true,"groupBy":[{"fieldName":"Deadline"}],"filter":{"conjunction":"and","conditions":[]}}`), "1..50"},
		{"snake filter field", fieldWith(`{"tableName":"Tasks","countAll":true,"groupBy":[{"fieldName":"Deadline"}],"filter":{"conjunction":"and","conditions":[{"field_name":"Done","operator":"is","value":false}]}}`), "field_name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: "POST", URL: "/blocks", Optional: true}
			reg.Register(stub)
			args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Invalid", "--type", "countdown"}
			if tc.config != "" {
				args = append(args, "--data-config", tc.config)
			}
			err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
			assertInvalidArgumentValidation(t, err, "--data-config", nil, tc.message)
			require.Empty(t, stub.CapturedBodies)
		})
	}
}

func TestDashboardCountdownArrayLimits(t *testing.T) {
	for _, size := range []int{20, 21, 50, 51} {
		var config map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(countdownFieldConfig), &config))
		source := config["additionalDatasources"].([]interface{})[0].(map[string]interface{})
		items := make([]interface{}, size)
		for i := range items {
			if size <= 21 {
				items[i] = map[string]interface{}{"fieldName": "Cost", "rollup": "SUM"}
			} else {
				items[i] = map[string]interface{}{"fieldName": "Done", "operator": "is", "value": false}
			}
		}
		if size <= 21 {
			delete(source, "countAll")
			source["series"] = items
		} else {
			source["filter"] = map[string]interface{}{"conjunction": "and", "conditions": items}
		}
		problems := validateBlockDataConfig("countdown", config)
		if size == 20 || size == 50 {
			require.Empty(t, problems)
		} else {
			require.NotEmpty(t, problems)
		}
	}
}

func TestDashboardCountdownNoValidateAndMalformedJSON(t *testing.T) {
	for _, shortcut := range []common.Shortcut{BaseDashboardBlockCreate, BaseDashboardBlockUpdate} {
		for _, raw := range []string{`null`, `[]`, `false`, `0`, `"fixed"`, `{"type":`} {
			t.Run(shortcut.Command+raw, func(t *testing.T) {
				factory, stdout, _ := newExecuteFactory(t)
				args := []string{shortcut.Command, "--base-token", "app_x", "--dashboard-id", "dsh_x", "--data-config", raw, "--no-validate", "--dry-run"}
				if shortcut.Command == BaseDashboardBlockCreate.Command {
					args = append(args, "--name", "Invalid", "--type", "countdown")
				} else {
					args = append(args, "--block-id", "blk_x")
				}
				err := runShortcut(t, shortcut, args, factory, stdout)
				assertInvalidArgumentValidation(t, err, "--data-config", nil, "")
			})
		}
	}
	// countdown is Dashboard-only; extending its validator must not widen App.
	require.False(t, isAppBlockType("countdown"))
}
