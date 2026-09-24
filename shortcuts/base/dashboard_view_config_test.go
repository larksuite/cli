// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const dashboardViewTestGrid = `{"table_name":"任务表","view_type":"grid","filter":{"conjunction":"and","conditions":[{"field_name":"状态","operator":"is","value":"进行中"}]},"group_by":[{"field_name":"状态","mode":"integrated","sort":{"type":"group","order":"asc"}}],"sort_by":[{"field_name":"截止时间","order":"asc"}],"visible_fields":["任务名称","状态","负责人","截止时间"]}`
const dashboardViewTestGantt = `{"table_name":"任务表","view_type":"gantt"}`
const dashboardViewTestGallery = `{"table_name":"任务表","view_type":"gallery"}`

// Read responses contain view references, not the writable dashboard config.
func dashboardViewTestReference(viewType string) string {
	return fmt.Sprintf(`{"tableId":"tbl_tasks","viewId":"vew_tasks","viewType":%q}`, viewType)
}

func TestDashboardViewCreateConfiguration(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, config := range []string{
		`{"table_name":"Tasks","view_type":"grid"}`,
		dashboardViewTestGrid, dashboardViewTestGantt, dashboardViewTestGallery,
		`{"table_name":"Tasks","view_type":"grid","group_by":[{"field_name":"A"},{"field_name":"B"},{"field_name":"C"}],"sort_by":[],"visible_fields":[]}`,
		`{"table_name":"Tasks","view_type":"grid","filter":{"conjunction":"and","conditions":[{"field_name":"Cost","operator":"is","value":0},{"field_name":"Done","operator":"is","value":false}]}}`,
		`{"table_name":"Tasks","view_type":"grid","sort_by":[{"field_name":"card","order":"asc"}],"visible_fields":["timebar","card"]}`,
	} {
		t.Run(config, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			viewType := gjson.Get(config, "view_type").String()
			responseConfig := dashboardViewTestReference(viewType)
			stub := &httpmock.Stub{Method: "POST", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_view","type":"view","data_config":` + responseConfig + `}}`)}
			reg.Register(stub)
			err := runShortcut(t, BaseDashboardBlockCreate, []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Task view", "--type", "view", "--data-config", config, "--position", `{"x":0,"y":0,"w":12,"h":8}`}, factory, stdout)
			require.NoError(t, err)
			require.JSONEq(t, config, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
			require.Equal(t, "view", gjson.GetBytes(stub.CapturedBody, "type").String())
			require.Equal(t, int64(12), gjson.GetBytes(stub.CapturedBody, "position.w").Int())
			require.False(t, gjson.GetBytes(stub.CapturedBody, "data_config.series").Exists())
			require.False(t, gjson.GetBytes(stub.CapturedBody, "data_config.count_all").Exists())
			require.Equal(t, "blk_view", gjson.Get(stdout.String(), "data.block.block_id").String())
			require.True(t, gjson.Get(stdout.String(), "data.created").Bool())
			require.JSONEq(t, responseConfig, gjson.Get(stdout.String(), "data.block.data_config").Raw)
			require.Equal(t, gjson.String, gjson.Get(stdout.String(), "data.block.data_config.viewType").Type)
		})
	}
}

func TestDashboardViewInvalidCreate(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct{ name, config, want string }{
		{"missing config", "", "table_name 和 view_type"},
		{"missing table", `{"view_type":"grid"}`, "table_name"},
		{"missing view type", `{"table_name":"Tasks"}`, "view_type"},
		{"blank view type", `{"table_name":"Tasks","view_type":" "}`, "view_type"},
		{"view type number", `{"table_name":"Tasks","view_type":1}`, "view_type"},
		{"chart metric", `{"table_name":"Tasks","view_type":"grid","count_all":true}`, "count_all"},
		{"chart series", `{"table_name":"Tasks","view_type":"grid","series":[]}`, "series"},
		{"unknown key", `{"table_name":"Tasks","view_type":"grid","view_id":"vew_x"}`, "view_id"},
		{"null groups", `{"table_name":"Tasks","view_type":"grid","group_by":null}`, "group_by"},
		{"group field missing", `{"table_name":"Tasks","view_type":"grid","group_by":[{}]}`, "field_name"},
		{"wrong group shape", `{"table_name":"Tasks","view_type":"grid","group_by":{}}`, "group_by"},
		{"null group", `{"table_name":"Tasks","view_type":"grid","group_by":[null]}`, "field_name"},
		{"invalid group sort", `{"table_name":"Tasks","view_type":"grid","group_by":[{"field_name":"Status","sort":{"type":"group","order":"sideways"}}]}`, "sort.order"},
		{"sort field missing", `{"table_name":"Tasks","view_type":"grid","sort_by":[{"order":"asc"}]}`, "field_name"},
		{"sort direction missing", `{"table_name":"Tasks","view_type":"grid","sort_by":[{"field_name":"Due"}]}`, "sort_by[0].order"},
		{"sort direction invalid", `{"table_name":"Tasks","view_type":"grid","sort_by":[{"field_name":"Due","order":false}]}`, "order"},
		{"null sort", `{"table_name":"Tasks","view_type":"grid","sort_by":null}`, "sort_by"},
		{"blank visible field", `{"table_name":"Tasks","view_type":"grid","visible_fields":[" "]}`, "visible_fields[0]"},
		{"wrong visible field shape", `{"table_name":"Tasks","view_type":"grid","visible_fields":[1]}`, "visible_fields"},
		{"null visible fields", `{"table_name":"Tasks","view_type":"grid","visible_fields":null}`, "visible_fields"},
		{"wrong filter protocol", `{"table_name":"Tasks","view_type":"grid","filter":{"logic":"and","conditions":[["Status","==","Done"]]}}`, "filter.conjunction"},
		{"filter value missing", `{"table_name":"Tasks","view_type":"grid","filter":{"conjunction":"and","conditions":[{"field_name":"Status","operator":"is"}]}}`, "value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Task view", "--type", "view"}
			if tc.config != "" {
				args = append(args, "--data-config", tc.config)
			}
			err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Equal(t, "--data-config", validation.Param)
			require.Contains(t, validation.Error(), tc.want)
			require.Empty(t, stdout.String())
		})
	}
}

func TestDashboardViewRejectsUnsupportedWriteFields(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		fields       []string
	}{
		{"timebar object", `{"timebar":{"start_time":"Start","end_time":"End","title":"Title"}}`, []string{"timebar"}},
		{"timebar empty", `{"timebar":{}}`, []string{"timebar"}},
		{"timebar null", `{"timebar":null}`, []string{"timebar"}},
		{"card object", `{"card":{"cover_field":"Cover"}}`, []string{"card"}},
		{"card empty", `{"card":{}}`, []string{"card"}},
		{"card null", `{"card":null}`, []string{"card"}},
		{"both fields", `{"timebar":null,"card":{"cover_field":null}}`, []string{"timebar", "card"}},
	} {
		for _, shortcut := range []common.Shortcut{BaseDashboardBlockCreate, BaseDashboardBlockUpdate} {
			t.Run(tc.name+"/"+shortcut.Command, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				factory, stdout, reg := newExecuteFactory(t)
				stub := &httpmock.Stub{URL: "/blocks", Optional: true, RawBody: []byte(`{"code":0,"data":{"block_id":"blk_view"}}`)}
				reg.Register(stub)
				args := []string{shortcut.Command, "--base-token", "app_x", "--dashboard-id", "dsh_x"}
				config := tc.config
				if shortcut.Command == "+dashboard-block-create" {
					args = append(args, "--name", "Task view", "--type", "view")
					config = `{"table_name":"Tasks","view_type":"grid",` + config[1:]
				} else {
					args = append(args, "--block-id", "blk_view")
				}
				err := runShortcut(t, shortcut, append(args, "--data-config", config), factory, stdout)
				var validation *errs.ValidationError
				require.ErrorAs(t, err, &validation)
				require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
				require.Equal(t, "--data-config", validation.Param)
				for _, field := range tc.fields {
					require.Contains(t, validation.Error(), "data_config."+field+" 不支持")
				}
				require.Empty(t, stdout.String())
				require.Empty(t, stub.CapturedBodies, "invalid input must not reach the API")
			})
		}
	}
}

func TestDashboardViewUpdateAndReadback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, patch := range []string{
		`{}`, `{"table_name":"Other tasks"}`, `{"view_type":"gallery"}`, `{"view_type":"gantt"}`,
		`{"filter":null}`, `{"filter":{"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":"Done"}]}}`,
		`{"group_by":[]}`, `{"group_by":[{"field_name":"Status","sort":{"type":"group","order":"desc"}}]}`,
		`{"sort_by":[]}`, `{"sort_by":[{"field_name":"Due","order":"desc"}]}`,
		`{"visible_fields":["Title","Due"]}`,
		`{"visible_fields":["timebar","card"]}`, dashboardViewTestGrid,
	} {
		t.Run(patch, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			viewType := "grid"
			if value := gjson.Get(patch, "view_type"); value.Exists() {
				viewType = value.String()
			}
			responseConfig := dashboardViewTestReference(viewType)
			stub := &httpmock.Stub{Method: "PATCH", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_view", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_view","type":"view","data_config":` + responseConfig + `}}`)}
			reg.Register(stub)
			require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_view", "--data-config", patch}, factory, stdout))
			require.JSONEq(t, patch, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
			require.False(t, gjson.GetBytes(stub.CapturedBody, "type").Exists())
			require.JSONEq(t, responseConfig, gjson.Get(stdout.String(), "data.block.data_config").Raw)
			require.Equal(t, gjson.String, gjson.Get(stdout.String(), "data.block.data_config.viewType").Type)
		})
	}
	for _, config := range []string{
		dashboardViewTestReference("grid"), dashboardViewTestReference("gantt"), dashboardViewTestReference("gallery"),
		`{"tableId":"tbl_tasks","viewId":"vew_tasks","viewType":"gantt","timebar":{"start_time":"开始时间","end_time":"结束时间","title":"任务名称"}}`,
		`{"tableId":"tbl_tasks","viewId":"vew_tasks","viewType":"gallery","card":{"cover_field":"封面"}}`,
	} {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_view", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_view","type":"view","data_config":` + config + `}}`)})
		require.NoError(t, runShortcut(t, BaseDashboardBlockGet, []string{"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_view"}, factory, stdout))
		require.Equal(t, "view", gjson.Get(stdout.String(), "data.block.type").String())
		require.JSONEq(t, config, gjson.Get(stdout.String(), "data.block.data_config").Raw)
		require.Equal(t, gjson.String, gjson.Get(stdout.String(), "data.block.data_config.viewType").Type)
	}
}

func TestDashboardViewFileAndNoValidate(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name, method, command, config string
		shortcut                      common.Shortcut
		extra                         []string
	}{
		{"create file", "POST", "+dashboard-block-create", dashboardViewTestGrid, BaseDashboardBlockCreate, []string{"--name", "Task view", "--type", "view"}},
		{"update file", "PATCH", "+dashboard-block-update", `{"visible_fields":["Title","Status"]}`, BaseDashboardBlockUpdate, []string{"--block-id", "blk_view"}},
		{"no validate", "POST", "+dashboard-block-create", `{"view_type":false,"future_option":0}`, BaseDashboardBlockCreate, []string{"--name", "Task view", "--type", "view", "--no-validate"}},
		{"no validate view create", "POST", "+dashboard-block-create", `{"table_name":"Tasks","view_type":"gantt","timebar":{"start_time":"Start","end_time":"End","title":"Title"},"card":null}`, BaseDashboardBlockCreate, []string{"--name", "Task view", "--type", "view", "--no-validate"}},
		{"no validate view update", "PATCH", "+dashboard-block-update", `{"timebar":null,"card":{"cover_field":null}}`, BaseDashboardBlockUpdate, []string{"--block-id", "blk_view", "--no-validate"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			withBaseWorkingDir(t, dir)
			path := filepath.Join(dir, "view.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.config), 0600))
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: tc.method, URL: "/blocks", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_view"}}`)}
			reg.Register(stub)
			args := append([]string{tc.command, "--base-token", "app_x", "--dashboard-id", "dsh_x", "--data-config", "@" + path}, tc.extra...)
			require.NoError(t, runShortcut(t, tc.shortcut, args, factory, stdout))
			require.JSONEq(t, tc.config, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
		})
	}
	require.False(t, isAppBlockType("view"))
	require.False(t, isChartBlockType("view"))
}
